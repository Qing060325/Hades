// Package pool Goroutine 池
package pool

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// ErrPoolClosed 池已关闭
var ErrPoolClosed = errors.New("pool is closed")

// Task 任务函数类型
type Task func()

// Pool Goroutine 池
type Pool struct {
	// 配置
	workers    int
	taskQueue  chan Task
	semaphore  chan struct{}
	wg         sync.WaitGroup

	// 状态
	closed     atomic.Bool
	taskCount  atomic.Int64
	workerCount atomic.Int32

	// 统计
	submitted atomic.Int64
	completed atomic.Int64
}

// PoolOption 池选项
type PoolOption func(*Pool)

// WithWorkers 设置工作协程数量
func WithWorkers(workers int) PoolOption {
	return func(p *Pool) {
		if workers > 0 {
			p.workers = workers
		}
	}
}

// WithQueueSize 设置任务队列大小
func WithQueueSize(size int) PoolOption {
	return func(p *Pool) {
		p.taskQueue = make(chan Task, size)
	}
}

// NewPool 创建 Goroutine 池
func NewPool(opts ...PoolOption) *Pool {
	p := &Pool{
		workers:   runtime.NumCPU() * 2,
		taskQueue: make(chan Task, 1000),
	}

	for _, opt := range opts {
		opt(p)
	}

	p.semaphore = make(chan struct{}, p.workers)
	p.start()

	return p
}

// start 启动工作协程
func (p *Pool) start() {
	// 预启动一些工作协程
	for i := 0; i < p.workers/2; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

// worker 工作协程
func (p *Pool) worker() {
	defer p.wg.Done()

	for task := range p.taskQueue {
		if task == nil {
			continue
		}

		p.taskCount.Add(-1)
		p.executeTask(task)
		<-p.semaphore
	}
}

// executeTask 执行任务
func (p *Pool) executeTask(task Task) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("error", r).Msg("[Pool] 任务执行panic")
		}
		p.completed.Add(1)
	}()

	task()
}

// Submit 提交任务
func (p *Pool) Submit(task Task) error {
	return p.SubmitWithContext(context.Background(), task)
}

// SubmitWithContext 提交任务（带上下文）
func (p *Pool) SubmitWithContext(ctx context.Context, task Task) error {
	if p.closed.Load() {
		return ErrPoolClosed
	}

	if task == nil {
		return nil
	}

	select {
	case p.semaphore <- struct{}{}:
		// 有空位，启动新协程
		p.submitted.Add(1)
		p.taskCount.Add(1)
		p.workerCount.Add(1)

		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Error().Interface("error", r).Msg("[Pool] 任务执行panic")
				}
				p.completed.Add(1)
				p.workerCount.Add(-1)
			}()
			task()
		}()
		return nil

	case p.taskQueue <- task:
		// 任务入队
		p.submitted.Add(1)
		p.taskCount.Add(1)
		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}

// TrySubmit 尝试提交任务（非阻塞）
func (p *Pool) TrySubmit(task Task) bool {
	if p.closed.Load() {
		return false
	}

	select {
	case p.taskQueue <- task:
		p.submitted.Add(1)
		p.taskCount.Add(1)
		return true
	default:
		return false
	}
}

// Close 关闭池
func (p *Pool) Close() {
	if p.closed.Swap(true) {
		return // 已经关闭
	}

	close(p.taskQueue)
	p.wg.Wait()
}

// Stats 获取统计信息
func (p *Pool) Stats() PoolStats {
	return PoolStats{
		Workers:    p.workers,
		QueueSize:  cap(p.taskQueue),
		QueueCount: int(p.taskCount.Load()),
		WorkerCount: int(p.workerCount.Load()),
		Submitted:  p.submitted.Load(),
		Completed:  p.completed.Load(),
	}
}

// PoolStats 池统计信息
type PoolStats struct {
	Workers     int
	QueueSize   int
	QueueCount  int
	WorkerCount int
	Submitted   int64
	Completed   int64
}

// IOPool IO密集型任务池
type IOPool struct {
	*Pool
}

// NewIOPool 创建IO密集型任务池
func NewIOPool() *IOPool {
	return &IOPool{
		Pool: NewPool(
			WithWorkers(runtime.NumCPU() * 8), // IO密集型使用更多协程
			WithQueueSize(2000),
		),
	}
}

// ComputePool 计算密集型任务池
type ComputePool struct {
	*Pool
}

// NewComputePool 创建计算密集型任务池
func NewComputePool() *ComputePool {
	return &ComputePool{
		Pool: NewPool(
			WithWorkers(runtime.NumCPU()), // 计算密集型使用CPU核心数
			WithQueueSize(500),
		),
	}
}

// defaultPool 默认池
var defaultPool = NewPool()

// Submit 提交任务到默认池
func Submit(task Task) error {
	return defaultPool.Submit(task)
}

// SubmitWithContext 提交任务到默认池（带上下文）
func SubmitWithContext(ctx context.Context, task Task) error {
	return defaultPool.SubmitWithContext(ctx, task)
}

// TrySubmit 尝试提交任务到默认池
func TrySubmit(task Task) bool {
	return defaultPool.TrySubmit(task)
}

// DefaultPool 获取默认池
func DefaultPool() *Pool {
	return defaultPool
}

// Schedule 定时调度器
type Schedule struct {
	interval time.Duration
	task     Task
	cancel   context.CancelFunc
}

// NewSchedule 创建定时任务
func NewSchedule(interval time.Duration, task Task) *Schedule {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Schedule{
		interval: interval,
		task:     task,
		cancel:   cancel,
	}

	go s.run(ctx)
	return s
}

func (s *Schedule) run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			Submit(s.task)
		case <-ctx.Done():
			return
		}
	}
}

// Stop 停止定时任务
func (s *Schedule) Stop() {
	s.cancel()
}
