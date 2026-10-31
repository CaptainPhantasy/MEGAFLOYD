package update

import "testing"

func TestReleaseSourceIsMegaFloyd(t *testing.T) {
	if githubApiUrl != "https://api.github.com/repos/CaptainPhantasy/MEGAFLOYD/releases/latest" {
		t.Fatalf("wrong release source: %s", githubApiUrl)
	}
	if userAgent != "MegaFloyd" {
		t.Fatalf("wrong updater identity: %s", userAgent)
	}
}
