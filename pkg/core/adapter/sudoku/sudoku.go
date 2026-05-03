// Package sudoku Sudoku 协议实现
// Sudoku 使用基于数独谜题的认证机制，通过 TCP 连接进行代理
package sudoku

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/perf/pool"
	"github.com/rs/zerolog/log"
)

const (
	// puzzleGridSize 数独网格大小 (9x9)
	puzzleGridSize = 9
	// puzzleGridCells 数独总格数
	puzzleGridCells = puzzleGridSize * puzzleGridSize
	// protocolMagic 协议魔数
	protocolMagic byte = 0x53 // 'S'
	// protocolVersion 协议版本
	protocolVersion byte = 0x01
	// commandConnect TCP CONNECT 命令
	commandConnect byte = 0x01
	// nonceSize 随机数长度
	nonceSize = 16
)

// Adapter Sudoku 适配器
type Adapter struct {
	name     string
	server   string
	port     int
	password string
	dialer   *net.Dialer
}

// NewAdapter 创建 Sudoku 适配器
func NewAdapter(name, server string, port int, password string) (*Adapter, error) {
	if password == "" {
		return nil, fmt.Errorf("Sudoku 密码不能为空")
	}

	return &Adapter{
		name:     name,
		server:   server,
		port:     port,
		password: password,
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}, nil
}

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeSudoku }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool { return false }

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return true }

// DialContext 建立连接
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	log.Debug().
		Str("server", a.Addr()).
		Str("target", metadata.DestinationAddress()).
		Msg("[Sudoku] 建立连接")

	// 连接服务器
	conn, err := a.dialer.DialContext(ctx, "tcp", a.Addr())
	if err != nil {
		return nil, fmt.Errorf("连接 Sudoku 服务器失败: %w", err)
	}

	// Sudoku 认证握手
	if err := a.authenticate(ctx, conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("Sudoku 认证失败: %w", err)
	}

	// 发送目标地址
	if err := a.sendTarget(conn, metadata); err != nil {
		conn.Close()
		return nil, fmt.Errorf("Sudoku 发送目标地址失败: %w", err)
	}

	return conn, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	return nil, fmt.Errorf("Sudoku 不支持 UDP")
}

// URLTest 健康检查
func (a *Adapter) URLTest(ctx context.Context, testURL string) (time.Duration, error) {
	start := time.Now()
	conn, err := a.DialContext(ctx, &adapter.Metadata{
		Host:    "www.gstatic.com",
		DstPort: 80,
	})
	if err != nil {
		return 0, err
	}
	conn.Close()
	return time.Since(start), nil
}

// authenticate Sudoku 认证握手
// 协议流程:
// 1. 客户端发送: [magic] [version] [16 bytes nonce]
// 2. 服务端返回: [9x9 puzzle grid] [81 bytes]
// 3. 客户端解决谜题并发送: [81 bytes solution] [32 bytes auth hash]
// 4. 服务端验证后返回: [1 byte status] (0x00 = 成功)
func (a *Adapter) authenticate(ctx context.Context, conn net.Conn) error {
	// 步骤1: 发送handshakeHeader
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("生成随机数失败: %w", err)
	}

handshakeHeader := make([]byte, 1+1+nonceSize)
	handshakeHeader[0] = protocolMagic
	handshakeHeader[1] = protocolVersion
	copy(handshakeHeader[2:], nonce)

	if _, err := conn.Write(handshakeHeader); err != nil {
		return fmt.Errorf("发送handshakeHeader失败: %w", err)
	}

	// 步骤2: 接收数独谜题
	puzzle := make([]byte, puzzleGridCells)
	if _, err := io.ReadFull(conn, puzzle); err != nil {
		return fmt.Errorf("接收数独谜题失败: %w", err)
	}

	// 验证谜题有效性
	if !isValidPuzzle(puzzle) {
		return fmt.Errorf("收到无效的数独谜题")
	}

	// 步骤3: 解决数独谜题
	solution := solvePuzzle(puzzle)
	if solution == nil {
		return fmt.Errorf("无法解决数独谜题")
	}

	// 计算认证哈希
	authHash := computeSolutionHash(solution, a.password, nonce)

	// 发送解答和认证哈希
	response := make([]byte, puzzleGridCells+32)
	copy(response[:puzzleGridCells], solution)
	copy(response[puzzleGridCells:], authHash[:])

	if _, err := conn.Write(response); err != nil {
		return fmt.Errorf("发送解答失败: %w", err)
	}

	// 步骤4: 接收验证结果
	status := make([]byte, 1)
	if _, err := io.ReadFull(conn, status); err != nil {
		return fmt.Errorf("接收验证结果失败: %w", err)
	}

	if status[0] != 0x00 {
		return fmt.Errorf("Sudoku 认证被拒绝: 状态码 0x%02x", status[0])
	}

	return nil
}

// sendTarget 发送目标地址
func (a *Adapter) sendTarget(conn net.Conn, metadata *adapter.Metadata) error {
	buf := pool.GetMedium()
	defer pool.Put(buf)
	offset := 0

	// 命令
	buf[offset] = commandConnect
	offset++

	// 目标地址 (SOCKS5 格式)
	addr := packAddr(metadata.Host, metadata.DstPort)
	copy(buf[offset:], addr)
	offset += len(addr)

	_, err := conn.Write(buf[:offset])
	return err
}

// isValidPuzzle 验证数独谜题有效性
func isValidPuzzle(puzzle []byte) bool {
	if len(puzzle) != puzzleGridCells {
		return false
	}

	// 检查是否有足够的预填格
	filled := 0
	for _, v := range puzzle {
		if v > 0 && v <= 9 {
			filled++
		}
	}

	// 数独最少需要 17 个已知格才有唯一解
	return filled >= 17
}

// computeSolutionHash 计算解答哈希
func computeSolutionHash(solution []byte, password string, nonce []byte) [32]byte {
	h := sha256.New()
	h.Write(solution)
	h.Write([]byte(password))
	h.Write(nonce)
	h.Write([]byte("sudoku-auth-salt"))

	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// solvePuzzle 解决数独谜题 (回溯法)
func solvePuzzle(puzzle []byte) []byte {
	// 复制谜题到 9x9 网格
	grid := make([][]byte, puzzleGridSize)
	for i := range grid {
		grid[i] = make([]byte, puzzleGridSize)
		copy(grid[i], puzzle[i*puzzleGridSize:(i+1)*puzzleGridSize])
	}

	// 回溯法求解
	if solveSudoku(grid) {
		// 转换回一维数组
		solution := make([]byte, puzzleGridCells)
		for i := 0; i < puzzleGridSize; i++ {
			copy(solution[i*puzzleGridSize:(i+1)*puzzleGridSize], grid[i])
		}
		return solution
	}

	return nil
}

// solveSudoku 回溯法求解数独
func solveSudoku(grid [][]byte) bool {
	row, col, found := findEmpty(grid)
	if !found {
		return true // 没有空格，已解决
	}

	for num := byte(1); num <= 9; num++ {
		if isValidPlacement(grid, row, col, num) {
			grid[row][col] = num

			if solveSudoku(grid) {
				return true
			}

			grid[row][col] = 0 // 回溯
		}
	}

	return false
}

// findEmpty 查找空格
func findEmpty(grid [][]byte) (int, int, bool) {
	for i := 0; i < puzzleGridSize; i++ {
		for j := 0; j < puzzleGridSize; j++ {
			if grid[i][j] == 0 {
				return i, j, true
			}
		}
	}
	return 0, 0, false
}

// isValidPlacement 检查数字放置是否有效
func isValidPlacement(grid [][]byte, row, col int, num byte) bool {
	// 检查行
	for j := 0; j < puzzleGridSize; j++ {
		if grid[row][j] == num {
			return false
		}
	}

	// 检查列
	for i := 0; i < puzzleGridSize; i++ {
		if grid[i][col] == num {
			return false
		}
	}

	// 检查 3x3 宫格
	boxRow := (row / 3) * 3
	boxCol := (col / 3) * 3
	for i := boxRow; i < boxRow+3; i++ {
		for j := boxCol; j < boxCol+3; j++ {
			if grid[i][j] == num {
				return false
			}
		}
	}

	return true
}

// packAddr 打包目标地址 (SOCKS5 格式)
func packAddr(host string, port uint16) []byte {
	buf := pool.GetSmall()
	defer pool.Put(buf)

	offset := 0
	ip := net.ParseIP(host)
	if ip == nil {
		domain := []byte(host)
		if len(domain) > 255 {
			domain = domain[:255]
		}
		buf[offset] = 0x03
		offset++
		buf[offset] = byte(len(domain))
		offset++
		copy(buf[offset:], domain)
		offset += len(domain)
	} else if ip4 := ip.To4(); ip4 != nil {
		buf[offset] = 0x01
		offset++
		copy(buf[offset:], ip4)
		offset += 4
	} else {
		buf[offset] = 0x04
		offset++
		copy(buf[offset:], ip.To16())
		offset += 16
	}

	binary.BigEndian.PutUint16(buf[offset:], port)
	offset += 2

	result := make([]byte, offset)
	copy(result, buf[:offset])
	return result
}

// unpackAddr 解包目标地址
func unpackAddr(data []byte) (string, uint16, []byte, error) {
	if len(data) < 1 {
		return "", 0, nil, io.ErrShortBuffer
	}

	var host string
	var offset int

	switch data[0] {
	case 0x01:
		if len(data) < 7 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = net.IP(data[1:5]).String()
		offset = 5
	case 0x03:
		if len(data) < 2 {
			return "", 0, nil, io.ErrShortBuffer
		}
		domainLen := int(data[1])
		if len(data) < 2+domainLen+2 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = string(data[2 : 2+domainLen])
		offset = 2 + domainLen
	case 0x04:
		if len(data) < 19 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = net.IP(data[1:17]).String()
		offset = 17
	default:
		return "", 0, nil, fmt.Errorf("未知地址类型: 0x%02x", data[0])
	}

	port := binary.BigEndian.Uint16(data[offset:])
	remaining := data[offset+2:]

	return host, port, remaining, nil
}
