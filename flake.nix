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
      in {
        devShells.default = mkGoDevShell {
          cmd = "cd cmd/${name} && go run main.go";
          hotReload = false;
          extraPackages = with pkgs; [
            nixpkgs-fmt
          ];
        };

        packages.default = pkgs.buildGoModule {
          inherit name;
          src = ./.;
          vendorHash = null;
          buildFlags = ["-mod=mod"];
          subPackages = ["cmd/${name}"];
          proxyVendor = true;
          allowGoReference = true;  # Allow Go to fetch from the network
          CGO_ENABLED = "0";  # Disable CGO for better compatibility
          # Explicitly set GOPROXY
          preBuild = ''
            export GOPROXY=https://proxy.golang.org,direct
          '';
        };
      };
    };
}
