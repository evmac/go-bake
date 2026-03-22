# typed: false
# frozen_string_literal: true
#
# Source of truth for evmac/homebrew-bake Formula/bake.rb.
# The release workflow copies this file and substitutes TAG and SHA256.

class Bake < Formula
  desc "Minimal Make replacement: one binary, one Bakefile, explicit DAG, typed args"
  homepage "https://github.com/evmac/go-bake"
  url "https://github.com/evmac/go-bake/archive/refs/tags/TAG_PLACEHOLDER.tar.gz"
  sha256 "SHA256_PLACEHOLDER"
  license "MIT"
  head "https://github.com/evmac/go-bake.git", branch: "trunk"

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X main.Version=v#{version}"
    system "go", "build", *std_go_args(ldflags: ldflags), "./cmd/bake"
  end

  test do
    (testpath/"Bakefile").write <<~BAKE
      target build { steps { exec ["true"] } }
    BAKE
    assert_match "build", shell_output("#{bin}/bake --list")
  end
end
