# Hades OpenWrt Package Makefile
# 放入 openwrt/package/hades/ 目录下使用

include $(TOPDIR)/rules.mk

PKG_NAME:=hades
PKG_VERSION:=1.0.0
PKG_RELEASE:=1

PKG_SOURCE_PROTO:=git
PKG_SOURCE_URL:=https://github.com/Qing060325/Hades.git
PKG_SOURCE_VERSION:=v1.0.0
PKG_MIRROR_HASH:=skip

PKG_LICENSE:=MIT
PKG_LICENSE_FILES:=LICENSE
PKG_MAINTAINER:=Qing060325

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1

GO_PKG:=github.com/Qing060325/Hades
GO_PKG_LDFLAGS:=-s -w
GO_PKG_LDFLAGS_X:=$(GO_PKG)/internal/version.Version=v$(PKG_VERSION) \
	$(GO_PKG)/internal/version.BuildTime=$(shell date -u '+%Y-%m-%d_%H:%M:%S') \
	$(GO_PKG)/internal/version.GoVersion=go1.25.0

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

define Package/hades
  SECTION:=net
  CATEGORY:=Network
  SUBMENU:=Proxy
  TITLE:=Hades - High performance proxy kernel
  URL:=https://github.com/Qing060325/Hades
  DEPENDS:=$(GO_ARCH_DEPENDS) +ca-bundle
endef

define Package/hades/description
  Hades is a high-performance proxy kernel written in Go.
  Compatible with Clash/Mihomo configuration format.
  Supports 18 proxy protocols including VMess, VLESS, Trojan,
  Hysteria2, TUIC, WireGuard, Shadowsocks and more.
endef

define Package/hades/conffiles
/etc/hades/config.yaml
/etc/config/hades
endef

define Build/Compile
	$(call GoPackage/Build/Compile)
endef

define Package/hades/install
	$(call GoPackage/Package/install-bin,$(1))

	# Init script
	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/hades.init $(1)/etc/init.d/hades

	# UCI config
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/hades.conf $(1)/etc/config/hades

	# Default config
	$(INSTALL_DIR) $(1)/etc/hades
	$(INSTALL_CONF) ./files/config.yaml $(1)/etc/hades/config.yaml
endef

define Package/hades/postinst
#!/bin/sh
[ -n "$${IPKG_INSTROOT}" ] || {
	/etc/init.d/hades enable
	echo "Hades installed. Edit /etc/hades/config.yaml then run:"
	echo "  /etc/init.d/hades start"
}
endef

define Package/hades/prerm
#!/bin/sh
[ -n "$${IPKG_INSTROOT}" ] || {
	/etc/init.d/hades stop 2>/dev/null
	/etc/init.d/hades disable 2>/dev/null
}
endef

$(eval $(call GoBinPackage,hades))
$(eval $(call BuildPackage,hades))
