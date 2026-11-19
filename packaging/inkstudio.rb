# The Homebrew cask, written to the tap by scripts/release.sh, which fills in
# @VERSION@ and @SHA256@. The app is signed and notarised, so the cask needs
# no quarantine workarounds.
cask "inkstudio" do
  version "@VERSION@"
  sha256 "@SHA256@"

  url "https://github.com/inkcheck/inkstudio/releases/download/v#{version}/InkStudio-#{version}.dmg"
  name "InkStudio"
  desc "Desktop editor and presenter for inkpress Markdown slides"
  homepage "https://github.com/inkcheck/inkstudio"

  livecheck do
    url :url
    strategy :github_latest
  end

  # inkline draws the diagrams in slides.
  depends_on cask: "inkcheck/tap/inkline"
  depends_on macos: :ventura

  app "InkStudio.app"

  zap trash: [
    "~/Library/Application Support/InkStudio",
    "~/Library/Caches/InkStudio",
    "~/Library/WebKit/InkStudio",
  ]

  caveats <<~EOS
    Exporting PDF, PowerPoint and OpenDocument files uses an installed
    Chrome, Chromium, Edge, Brave or Vivaldi.
  EOS
end
