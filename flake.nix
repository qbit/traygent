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
            buildGo121Module rec {
              pname = "traygent";
              version = "v0.0.0";
              src = ./.;

              vendorHash =
                "sha256-DEw+3jjClMlWhtwDndd8pXjiEU2//gvd8LLlgWf6KHw=";
              proxyVendor = true;

              nativeBuildInputs = [ pkg-config copyDesktopItems ];
              buildInputs = [
                glfw
                libGL
                libGLU
                openssh
                pkg-config
                xorg.libXcursor
                xorg.libXi
                xorg.libXinerama
                xorg.libXrandr
                xorg.libXxf86vm
                xorg.xinput
              ];

              desktopItems = [
                (makeDesktopItem {
                  name = "YayAgent";
                  exec = pname;
                  icon = pname;
                  desktopName = pname;
                })
              ];

              #postInstall = ''
              #  mkdir -p $out/share/
              #  cp -r icons $out/share/
              #'';
            };
        });

      defaultPackage = forAllSystems (system: self.packages.${system}.traygent);
      devShells = forAllSystems (system:
        let pkgs = nixpkgsFor.${system};
        in {
          default = pkgs.mkShell {
            shellHook = ''
              PS1='\u@\h:\@; '
              nix run github:qbit/xin#flake-warn
              echo "Go `${pkgs.go_1_21}/bin/go version`"
            '';
            buildInputs = with pkgs; [
              git
              go_1_21
              gopls
              go-tools

              glfw
              pkg-config
              xorg.libXcursor
              xorg.libXi
              xorg.libXinerama
              xorg.libXrandr
              xorg.libXxf86vm
              xorg.xinput
              graphviz

              go-font
            ];
          };
        });
    };
}
