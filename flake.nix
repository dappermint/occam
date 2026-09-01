{
  description = "native macOS control of a Razer BlackShark V3 Pro, without Synapse";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs =
    { nixpkgs, ... }:
    let
      inherit (nixpkgs) lib;
      systems = [
        "aarch64-darwin"
        "x86_64-darwin"
      ];
      forAllSystems = f: lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (
        pkgs:
        let
          occam = pkgs.buildGoModule {
            pname = "occam";
            version = "0.4.3";
            src = lib.fileset.toSource {
              root = ./.;
              fileset = lib.fileset.unions [
                ./go.mod
                ./go.sum
                ./main.go
                ./cmd
                ./internal
              ];
            };
            vendorHash = "sha256-BTRjk/nocKp/rQk0/vfB+GvHNgRsj95HszZbYdUWPUk=";
            env.CGO_ENABLED = 1;
            ldflags = [
              "-s"
              "-w"
              "-X github.com/dappermint/occam/cmd.version=0.4.3"
            ];
            meta = {
              description = "native macOS control of a Razer BlackShark V3 Pro, without Synapse";
              homepage = "https://github.com/dappermint/occam";
              license = lib.licenses.mit;
              mainProgram = "occam";
              platforms = lib.platforms.darwin;
            };
          };
        in
        {
          inherit occam;
          default = occam;
        }
      );

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gotools
            go-tools
            just
          ];
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
