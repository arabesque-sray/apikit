{
  description = "Flake for apikit";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = {
    self,
    nixpkgs,
    flake-utils,
  }:
    flake-utils.lib.eachDefaultSystem (system: let
      pkgs = nixpkgs.legacyPackages.${system};

      # Build-time metadata
      gitCommit = self.rev or "dirty";
      buildTime = builtins.toString self.lastModifiedDate or "unknown";

      gitTag = "v1.0.0";
      gitBranch = "master";

      updateTagScript = pkgs.writeScriptBin "update_tag" ''
        #!${pkgs.bash}/bin/bash
        GIT_TAG=$(git describe --abbrev=0 --tags)
        sed -i "s/gitTag = \".*\"/gitTag = \"$GIT_TAG\"/" flake.nix
        echo "Updated flake.nix gitTag to $GIT_TAG"
      '';

      buildInfoFlags = [
        "-X github.com/ExperienceOne/apikit/internal/framework/version.GitCommit=${gitCommit}"
        "-X github.com/ExperienceOne/apikit/internal/framework/version.GitBranch=${gitBranch}"
        "-X github.com/ExperienceOne/apikit/internal/framework/version.GitTag=${gitTag}"
        "-X github.com/ExperienceOne/apikit/internal/framework/version.BuildTime=${buildTime}"
      ];
    in {
      packages.default = pkgs.buildGoModule {
        pname = "apikit";
        version = "0.1.0";

        src = ./.;

        vendorHash = "sha256-o80A1b3478fG1sQc71fubEt6lNwiOi3ZMa1Xq4y//Nc=";
        # NOTE requires regenerating it when dependencies change

        nativeBuildInputs = [pkgs.git];

        preBuild = ''
          # Build fpacker first
          go build -ldflags "${builtins.concatStringsSep " " buildInfoFlags}" -o fpacker ./cmd/fpacker/main.go

          # Use fpacker to generate framework code
          ./fpacker -src ./internal/framework/ -dest ./framework/framework_code.go
          ./fpacker -src ./internal/framework/ -dest ./framework/framework_code_client.go -exclude=xserver,validation,middleware,unmarshal -kind=client
          ./fpacker -src ./internal/framework/ -dest ./framework/framework_code_server.go -exclude=xclient,roundtripper,hooks -kind=server
        '';

        buildPhase = ''
          runHook preBuild
          go build -ldflags "${builtins.concatStringsSep " " buildInfoFlags}" -o apikit ./cmd/apikit/main.go
        '';

        installPhase = ''
          mkdir -p $out/bin
          cp apikit $out/bin/
        '';
      };

      devShells.default = pkgs.mkShell {
        name = "apikit Dev Environment";

        packages = with pkgs; [
          # tools
          gnumake
          git
          docker

          # golang
          go
          gopls
          govulncheck

          # custom
          updateTagScript
        ];

        # Optional: Add any shell hooks or environment variables needed for development
        shellHook = ''
          echo "Welcome to apikit development environment"
          echo "Available commands:"
          echo "  update_tag    - Update flake.nix gitTag from git tags"
        '';
      };
    });
}
