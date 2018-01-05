require "language/go"

class Sshportal < Formula
  desc "sshportal: simple, fun and transparent SSH bastion"
  homepage "https://github.com/moul/sshportal"
  url "https://github.com/moul/sshportal/archive/v1.7.1.tar.gz"
  sha256 "4611ae2f30cc595b2fb789bd0c92550533db6d4b63c638dd78cf85517b6aeaf0"
  head "https://github.com/moul/sshportal.git"

  depends_on "go" => :build

  def install
    ENV["GOPATH"] = buildpath
    ENV["GOBIN"] = buildpath
    (buildpath/"src/github.com/moul/sshportal").install Dir["*"]

    system "go", "build", "-o", "#{bin}/sshportal", "-v", "github.com/moul/sshportal"
  end

  test do
    output = shell_output(bin/"sshportal --version")
    assert output.include? "sshportal version "
  end
end
