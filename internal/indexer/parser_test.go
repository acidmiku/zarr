package indexer

import "testing"

func TestAnimeBluRaySourceAliases(t *testing.T) {
	for _, tc := range []struct{ title, quality string }{
		{"[Group] Chainsaw Man - 01 [BD 1080p 10bit]", "bluray-1080p"},
		{"[Group] Chainsaw Man - 01 [1080p BDRip]", "bluray-1080p"},
		{"Chainsaw_Man_1080p_BluRay", "bluray-1080p"},
		{"Chainsaw.Man.[BluRay_1080p]", "bluray-1080p"},
		{"Chainsaw_Man_BD_1080p", "bluray-1080p"},
		{"Chainsaw.Man.2160p.Blu-Ray", "bluray-2160p"},
		{"Chainsaw.Man.[BD][2160p]", "bluray-2160p"},
		{"Chainsaw.Man.1080p.BD.Remux", "remux-1080p"},
		{"[SomeBDGroup] Chainsaw Man - 01 [1080p WEB-DL]", "web-1080p"},
		{"[Group] Archive [BD 720p]", "bluray-720p"},
		{"[Group] Archive [576p BluRay]", "bluray-576p"},
		{"[Group] Archive [480p BDRip]", "bluray-480p"},
		{"Archive.480p.WEBRip", "web-480p"},
		{"Archive.DVDRip", "dvd"},
		{"Archive.SDTV", "sdtv"},
		{"Archive.2160p.HDTV", "hdtv-2160p"},
	} {
		t.Run(tc.title, func(t *testing.T) {
			if got := ParseReleaseName(tc.title).Quality; got != tc.quality {
				t.Fatalf("quality=%q, want %q", got, tc.quality)
			}
		})
	}
}
