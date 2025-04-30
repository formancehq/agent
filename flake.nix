{
  description = "A Nix-flake-based Go 1.23 development environment";

  inputs = {
    nixpkgs.url = "https://flakehub.com/f/NixOS/nixpkgs/0.1.*.tar.gz";

    nur = {
      url = "github:nix-community/NUR";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, nur }:
    let
      goVersion = 23;

      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forEachSupportedSystem = f:
        nixpkgs.lib.genAttrs supportedSystems (system:
          let
            pkgs = import nixpkgs {
              inherit system;
              overlays = [ self.overlays.default nur.overlays.default ];
              config.allowUnfree = true;
            };
          in
          f { pkgs = pkgs; system = system; }
        );

    in
    {
      overlays.default = final: prev: {
        go = final."go_1_${toString goVersion}";
      };

      packages = forEachSupportedSystem ({ pkgs, system }:
        {

        }
      );

      devShells = forEachSupportedSystem ({ pkgs, system }:
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go
              gotools
              go-tools
              golangci-lint
              ginkgo
              yq-go
              jq
              pkgs.nur.repos.goreleaser.goreleaser-pro
              mockgen
              gomarkdoc
              just
              nodejs_22
              goperf
              protobuf
              protoc-gen-go
              protoc-gen-go-grpc
              openapi-generator-cli
            ];
          };
        }
      );
    };
}