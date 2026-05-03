{
  description = "Hades - High-performance proxy kernel";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      # Support common Go platforms
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
    in
    flake-utils.lib.eachSystem systems (system:
      let
        pkgs = import nixpkgs { inherit system; };

        version = if self ? rev then builtins.substring 0 8 self.rev else "dirty";

        hades = pkgs.buildGoModule {
          pname = "hades";
          inherit version;
          src = ./.;

          vendorHash = null; # Set to actual hash after first build, or use `null` with `go mod vendor`

          subPackages = [ "cmd/hades" ];

          ldflags = [
            "-s"
            "-w"
            "-X github.com/Qing060325/Hades/internal/version.Version=${version}"
            "-X github.com/Qing060325/Hades/internal/version.BuildTime=1970-01-01_00:00:00"
          ];

          CGO_ENABLED = 0;

          meta = with pkgs.lib; {
            description = "High-performance proxy kernel written in Go";
            homepage = "https://github.com/Qing060325/Hades";
            license = licenses.mit;
            maintainers = [ ];
            platforms = platforms.linux ++ platforms.darwin;
          };
        };
      in
      {
        packages = {
          default = hades;
          hades = hades;

          # Docker image
          docker = pkgs.dockerTools.buildLayeredImage {
            name = "ghcr.io/qing060325/hades";
            tag = version;
            contents = [ hades ];
            config = {
              Cmd = [ "/bin/hades" "-c" "/etc/hades/config.yaml" ];
              ExposedPorts = {
                "7890/tcp" = {}; # mixed-port
                "9090/tcp" = {}; # external-controller
              };
              Volumes = {
                "/etc/hades" = {};
              };
            };
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_25
            gopls
            gotools
            go-tools
            golangci-lint
            delve
          ];

          shellHook = ''
            echo "🔱 Hades development shell"
            echo "Go version: $(go version)"
          '';
        };
      }
    ) // {
      # NixOS module
      nixosModules.default = { config, lib, pkgs, ... }:
        let
          cfg = config.services.hades;
          hadesPkg = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
        in
        {
          options.services.hades = {
            enable = lib.mkEnableOption "Hades proxy kernel";

            package = lib.mkOption {
              type = lib.types.package;
              default = hadesPkg;
              description = "Hades package to use";
            };

            configFile = lib.mkOption {
              type = lib.types.path;
              description = "Path to Hades configuration file";
            };

            user = lib.mkOption {
              type = lib.types.str;
              default = "hades";
              description = "User to run Hades as";
            };

            group = lib.mkOption {
              type = lib.types.str;
              default = "hades";
              description = "Group to run Hades as";
            };

            openFirewall = lib.mkOption {
              type = lib.types.bool;
              default = false;
              description = "Open firewall ports for Hades";
            };

            ports = lib.mkOption {
              type = lib.types.listOf lib.types.port;
              default = [ 7890 9090 ];
              description = "Ports to open in the firewall";
            };
          };

          config = lib.mkIf cfg.enable {
            users.users.${cfg.user} = {
              isSystemUser = true;
              group = cfg.group;
              description = "Hades proxy service user";
            };
            users.groups.${cfg.group} = {};

            systemd.services.hades = {
              description = "Hades Proxy Kernel";
              after = [ "network-online.target" ];
              wants = [ "network-online.target" ];
              wantedBy = [ "multi-user.target" ];

              serviceConfig = {
                ExecStart = "${cfg.package}/bin/hades -c ${cfg.configFile}";
                Restart = "on-failure";
                RestartSec = 5;
                User = cfg.user;
                Group = cfg.group;
                CapabilityBoundingSet = [ "CAP_NET_ADMIN" "CAP_NET_BIND_SERVICE" "CAP_NET_RAW" ];
                AmbientCapabilities = [ "CAP_NET_ADMIN" "CAP_NET_BIND_SERVICE" "CAP_NET_RAW" ];
                NoNewPrivileges = true;
                LimitNOFILE = 65535;
              };
            };

            networking.firewall.allowedTCPPorts = lib.mkIf cfg.openFirewall cfg.ports;
            networking.firewall.allowedUDPPorts = lib.mkIf cfg.openFirewall cfg.ports;
          };
        };
    };
}
