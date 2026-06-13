{
  description = "Flake for chi-openapi development";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
  inputs.nixpkgs-unstable.url = "github:NixOS/nixpkgs/nixos-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";

  outputs = {
    self,
    nixpkgs,
    nixpkgs-unstable,
    flake-utils,
  }:
    flake-utils.lib.eachDefaultSystem
    (
      system: let
        stable = nixpkgs.legacyPackages.${system};
        unstable = nixpkgs-unstable.legacyPackages.${system};
      in {
        devShells.default = stable.mkShell {
          packages = with stable; [
            # go
            unstable.go
            unstable.golangci-lint
            unstable.gomodifytags
            unstable.gopls
            unstable.gotools
            unstable.go-tools
            unstable.gotests
            unstable.delve
            unstable.impl
            unstable.air
            unstable.gotestsum
          ];
        };
      }
    );
}
