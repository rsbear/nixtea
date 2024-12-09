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

          # Remove deprecated buildFlags
          tags = [""];  # Add any necessary build tags here
          ldflags = ["-s" "-w"];
          
          allowGoReference = true;  # Allow Go to fetch from the network
          proxyVendor = true;
          
          # Enable CGO for sqlite support
          CGO_ENABLED = "1";
          
          # Add sqlite to build inputs
          nativeBuildInputs = with pkgs; [ 
            pkg-config
            sqlite
          ];
          
          # Create SSH directory and generate key during build
          preBuild = ''
            export GOPROXY=https://proxy.golang.org,direct
            
            # Create SSH directory in the package
            mkdir -p $out/etc/nixtea/ssh
            
            # Generate SSH host key
            ${pkgs.openssh}/bin/ssh-keygen -t ed25519 -f $out/etc/nixtea/ssh/id_ed25519 -N ""
            
            # Set proper permissions
            chmod 755 $out/etc
            chmod 755 $out/etc/nixtea
            chmod 700 $out/etc/nixtea/ssh
            chmod 600 $out/etc/nixtea/ssh/id_ed25519
          '';

          # Update the host key path in the code
          postBuild = ''
            substituteInPlace $GOPATH/bin/${name} \
              --replace '.ssh/id_ed25519' '/etc/nixtea/ssh/id_ed25519'
          '';
        };
      };
    };
}
