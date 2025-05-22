{
  description = "traygent: a graphical ssh-agent";

  inputs.nixpkgs.url = "nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      supportedSystems =
        [ "x86_64-linux" "x86_64-darwin" "aarch64-linux" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
      nixpkgsFor = forAllSystems (system: import nixpkgs { inherit system; });
    in {
      packages = forAllSystems (system:
        let pkgs = nixpkgsFor.${system};

        in {
          traygent = with pkgs;
            buildGoModule rec {
              pname = "traygent";
              version = "v1.1.3";
              src = ./.;

              vendorHash = "sha256-tX/D/CJ+Fr+DkTKlHv4UxlRSwaBsOrr7buPp4Q3ypFU=";

              proxyVendor = true;

              nativeBuildInputs = [ pkg-config copyDesktopItems ];
              buildInputs = [
                fyne
                glfw
                libGL
                libGLU
                openssh
                pkg-config
                glibc
                xorg.libXcursor
                xorg.libXi
                xorg.libXinerama
                xorg.libXrandr
                xorg.libXxf86vm
                xorg.xinput

                wayland
                libxkbcommon
              ];

              # No wayland yet, it opens a second window
              buildPhase = ''
                ${fyne}/bin/fyne package --release
              '';

              installPhase = ''
                mkdir -p $out
                pkg="$PWD/${pname}.tar.xz"
                cd $out
                tar --strip-components=1 -xvf $pkg
              '';
            };
        });

      defaultPackage = forAllSystems (system: self.packages.${system}.traygent);
      devShells = forAllSystems (system:
        let pkgs = nixpkgsFor.${system};
        in {
          default = pkgs.mkShell {
            shellHook = ''
              PS1='\u@\h:\@; '
              export GOEXPERIMENT=loopvar
              nix run github:qbit/xin#flake-warn
              echo "Go `${pkgs.go}/bin/go version`"
            '';
            buildInputs = with pkgs; [
              fyne
              git
              go
              gopls
              go-tools
              glxinfo
              nilaway

              glfw
              glibc
              pkg-config
              xorg.libXcursor
              xorg.libXi
              xorg.libXinerama
              xorg.libXrandr
              xorg.libXxf86vm
              xorg.xinput
              graphviz

              wayland
              libxkbcommon

              go-font
            ];
          };
        });
    };
}

