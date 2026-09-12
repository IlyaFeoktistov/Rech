# Homebrew formula для rech - в отличие от installer/unix/install.sh
# (собирает и только ПРОВЕРЯЕТ внешние библиотеки), depends_on ниже
# заставляет brew САМ поставить все нужные библиотеки перед сборкой.
#
# НЕ ПРОВЕРЕНО на настоящем macOS/Homebrew (в этом окружении их нет) - ни
# синтаксис (ruby -c), ни `brew audit`, ни реальная установка. Собрана по
# документированному формату Homebrew formula, тем же уровнем доверия, что
# и остальные непроверенные Windows-ветки installer/windows/setup.nsi (см.
# README.md).
#
# Релизных тегов у проекта пока нет - только HEAD-сборка (нет url/sha256
# на tarball). Как только появится тег вида vX.Y.Z, formula стоит
# дополнить настоящими url/sha256 для стабильной (не --HEAD) установки.
#
# Установка (после публикации репозитория):
#   brew install --HEAD installer/macos/rech.rb
class Rech < Formula
  desc "Компилятор языка программирования Rech (транспилирует в C)"
  homepage "https://github.com/IlyaFeoktistov/Rech"
  license "MIT"
  head "https://github.com/IlyaFeoktistov/Rech.git", branch: "main"

  depends_on "go" => :build
  depends_on "bdw-gc"
  depends_on "openssl@3"
  depends_on "nghttp2"
  depends_on "libpng"
  depends_on "jpeg-turbo"
  depends_on "sqlite"
  depends_on "libpq"
  depends_on "mysql-client"

  def install
    version_str = "#{version}"
    version_str = "0.1.0+git#{Utils.git_head[0, 7]}" if build.head?
    system "go", "build", "-ldflags",
           "-X github.com/IlyaFeoktistov/Rech/internal/version.Version=#{version_str}",
           "-o", bin/"rech", "./cmd/rech"

    # Часовые пояса (IANA tzdata) - "<prefix>/share/rech/zoneinfo", тот же
    # путь, что вычисляет tzdataDir() (internal/backend/cbackend/tzdata.go)
    # для любого Unix-бинарника, установленного в "<префикс>/bin" -
    # Homebrew кладёт bin/ и share/ в один и тот же keg-префикс.
    (share/"rech/zoneinfo").install Dir["runtime/zoneinfo/*"]
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/rech version")
  end
end
