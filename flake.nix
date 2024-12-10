{
  description = "Example kickstart Go module project.";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
    devshells.url = "git://5.161.202.250/nix-dev-shells";
    devshells.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = inputs @ {flake-parts, ...}:
    flake-parts.lib.mkFlake {inherit inputs;} {
      systems = ["x86_64-linux" "aarch64-linux" "aarch64-darwin" "x86_64-darwin"];
      perSystem = {
        config,
        self',
        inputs',
        pkgs,
        system,
        ...
      }: let
        name = "nixtea";
        version = "latest";
        vendorHash = null; # update whenever go.mod changes
        mkGoDevShell = inputs.devshells.lib.${system}.mkGoDevShell;

        env = {
          local = {
            HOST = "localhost";
            PORT = "23234";
            HOST_KEY_PATH = "/tmp/nixtea/ssh/id_ed25519";
            DB_DIR = "/tmp";
            DB_NAME = "nixtea.db";
          };
          prod = {
            HOST = "0.0.0.0";
            PORT = "23234";
            HOST_KEY_PATH = "/etc/nixtea/ssh/id_ed25519";
            DB_DIR = "/var/lib/nixtea";
            DB_NAME = "nixtea.db";
          };
        };

        # Helper function to create --set arguments for wrapProgram
        mkSetFlags = vars: builtins.concatStringsSep " " (
          builtins.attrValues (builtins.mapAttrs (name: value: "--set ${name} \"${value}\"") vars)
        );

        # Helper function to create export statements
        mkExports = vars: builtins.concatStringsSep "\n" (
          builtins.attrValues (builtins.mapAttrs (name: value: "export ${name}=\"${value}\"") vars)
        );
      in {
        devShells.default = mkGoDevShell {
          cmd = "cd cmd/${name} && go run main.go";
          hotReload = false;
          extraPackages = with pkgs; [
            nixpkgs-fmt
          ];
          env = env.local;
        };

        packages.default = pkgs.buildGoModule {
          inherit name;
          src = ./.;
          vendorHash = null;
          buildFlags = ["-mod=mod"];
          subPackages = ["cmd/${name}"];

          # Use tags and ldflags instead of buildFlags
          tags = [""];
          ldflags = [ "-s" "-w" ];
          
          proxyVendor = true;
          
          # Enable CGO for sqlite support
          CGO_ENABLED = "1";
          
          # Add build dependencies
          nativeBuildInputs = with pkgs; [ 
            pkg-config
            makeWrapper
          ];

          # Set GOPROXY
          preBuild = ''
            export GOPROXY=https://proxy.golang.org,direct
          '';

          postFixup = ''
            wrapProgram $out/bin/${name} \
              ${mkSetFlags env.prod}
          '';

        };
      };
    };
}
