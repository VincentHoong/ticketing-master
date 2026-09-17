{
    description = "Ticketing Master dev environment";

    inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
    inputs.flake-parts.url = "github:hercules-ci/flake-parts";

    outputs = inputs@{ self, nixpkgs, flake-parts, ... }:
        flake-parts.lib.mkFlake { inherit inputs; } {
            systems = [ "x86_64-linux" "aarch64-darwin" "x86_64-darwin" "aarch64-linux" ];
            perSystem = { system, ... }:
            let
                pkgs = import nixpkgs { 
                    inherit system; 
                    config.allowUnfree = true; 
                };
                buildInputsCommon = [
                    pkgs.go_1_26
                ];
                buildInputsCI = buildInputsCommon;
                buildInputsDefault = buildInputsCommon ++ [
                    pkgs.gopls
                    pkgs.delve
                    pkgs.go-migrate
                ];
                mkDevShell = { buildInputs, extraShellHook ? "" }: pkgs.mkShellNoCC {
                    inherit buildInputs;
                    shellHook = ''
                        export CGO_ENABLED=0
                    '' + extraShellHook;
                };
                buildGoApp = pkgs.buildGo126Module {
                    pname = "ticketing-master";
                    version = "0.1.0";
                    src = ./backend;
                    vendorHash = "sha256-BhX9wHvFqfvPnRVpDJGUazSlRRRdxDi7fitn0yMA2xw=";
                };
            in {
                devShells.default = mkDevShell {
                    buildInputs = buildInputsDefault;
                    extraShellHook = ''
                        echo "Go $(go version | cut -d' ' -f3) dev shell ready"
                    '';
                };
                devShells.ci = mkDevShell {
                    buildInputs = buildInputsCI;
                };

                packages.default = buildGoApp;

                checks.default = buildGoApp.overrideAttrs (old: {
                    pname = "${old.pname}-vet";
                    dontBuild = true;
                    doCheck = true;
                    checkPhase = ''
                        export HOME=$TMPDIR
                        export GOCACHE=$TMPDIR/go-cache
                        runHook preCheck
                        go vet ./...
                        go tool golangci-lint run
                        runHook postCheck
                    '';
                    installPhase = "mkdir -p $out";
                });
            };
        };
}