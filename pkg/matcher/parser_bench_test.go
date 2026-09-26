package matcher_test

import (
	"fmt"
	"testing"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
)

// benchNames is synthetic; none of these come from a real library.
var benchNames = []string{
	"Test.Movie.Alpha.2023.2160p.UHD.Remux.HEVC.TrueHD.Atmos-FLUX.mkv",
	"The.Test.Film.Beta.2008.1080p.BluRay.x264.DTS-WiKi.mkv",
	"Test.Sequel.Gamma.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR10+.H.265-FLUX.mkv",
	"Test.Colony.Theta.2049.2017.1080p.BluRay.x264.DDP5.1.mkv",
	"2001.A.Test.Feature.Iota.1968.2160p.UHD.BluRay.x265.mkv",
	"1863.2019.1080p.BluRay.x264.DTS.mkv",
	"Test.Future.2077.No.Year.1080p.mkv",
	"The.Test.Feature.Kappa.2001.EXTENDED.2160p.UHD.BluRay.x265.TrueHD.7.1.Atmos-FLUX.mkv",
	"Test-Feature.Xi.Across.the.Line.2023.1080p.WEBRip.x264.AAC5.1.mkv",
	"Test_Feature_Omicron_2014_1080p_BluRay_x264_DTS.mkv",
	"Test.Feature.Pi.2022.2160p.UHD.BluRay.iso",
	"Test.Feature.Rho.2022.1080p.AV1.FLAC.mkv",
	"Test.Feature.Sigma.1999.DVDRip.XviD.AC3-EVO.avi",
	"Test.Feature.Tau.2023.2160p.WEB-DL.DDP5.1.Atmos.DoVi.HEVC-CMRG.mkv",
	"Test.Feature.Upsilon.2009.1080p.BluRay.x264.DTS-HD.MA.5.1.mkv",
	"12.Test.Figures.Lambda.1957.1080p.Criterion.BluRay.x264.FLAC-EA.mkv",
	"Test.Feature.Phi.2023.2160p.UHD.BluRay.TrueHD.7.1.Atmos.DV.HDR10.HEVC-FLUX.mkv",
	"Test.Feature.Chi.2019.1080p.BluRay.x264.AAC.mkv",
	"Test.Feature.Psi.2021.720p.HDTV.x264.mkv",
	"Test.Feature.Omega.2010.1080p.BluRay.DTS-HD.MA.mkv",

	"Test.Crime.Series.Delta.S01E05.Episode.Name.1080p.BluRay.x264-DEMAND.mkv",
	"Test.Show.Epsilon.S01E01-E02.1080p.BluRay.x265.mkv",
	"Test.Corporate.Drama.Zeta.S02E01-03.1080p.WEB-DL.mkv",
	"Test.Series.Eta.01x05.Episode.Title.720p.HDTV.x264.mkv",
	"The.Test.Series.Theta.Season.1.Episode.8.2160p.WEB-DL.mkv",
	"/downloads/complete/The.Test.Series.Iota.S02E06.Episode.1080p.HULU.WEB-DL.DDP5.1.H.264-FLUX.mkv",
	"Test.Series.Kappa.S01E10.Episode.Title.2160p.UHD.HDR.H.265.mkv",
	"The.Test.Series.Lambda.S01E03.Episode.Title.1080p.MAX.WEBRip.x265.mkv",
	"Test.Series.Xi.S06E13.Episode.Title.720p.HDTV.x264.mkv",
	"Test.Series.Chi.2019.S01E03.1080p.WEB-DL.x264.mkv",
	"Test Show Nu Season 2 Episode 14 1080p WEB-DL.mkv",
	"Test Show Sigma - Season 3 Episode 07 - 720p HDTV.mkv",
	"Test.Show.Omega.S03E12.mkv",
	"Test.Show.Rho.S10E22.Series.Finale.1080p.WEB-DL.mkv",
	"Test.Show.Tau.S01E01.720p.HDTV.mkv",

	"[Test-Group] Test Anime Omicron - 24 [1080p][HEVC][AAC].mkv",
	"[Test-Group] Test Anime Pi - 12 (1080p) [9A71B52F].mkv",
	"Test Anime Mu - 75 [1080p].mp4",
	"Test Anime Nu - 01 [1080p].mkv",
	"[SubGroup] Test Anime Upsilon - 07 [1080p][x265][10bit].mkv",
	"[SubGroup] Test Anime Phi - 03v2 [720p].mkv",

	"www.TestIndexer.org - Test Feature Chi (2019) 1080p BluRay x264 AAC.mkv",
	"www.TestTracker.company - Test.Series.Chi.S02E05.1080p.WEB-DL.mkv",
	"www.TestSite.net - Test Feature Alpha Two (2021) 2160p WEB-DL x265.mkv",
	"www.AnotherTestSite.io - Test.Show.Beta.S04E09.1080p.WEB-DL.mkv",

	"【测试发布组 www.TestSite.com】测试剧集[第01集][简繁英字幕] Test Series Omega 2024 S01E01 1080p WEB-DL H265 AAC.mkv",
	"【测试小组】Test.Feature.Delta.2022.1080p.WEB-DL.mkv",
	"测试剧集.S01E01.1080p.WEB-DL.mkv",
	"测试电影名称.2020.1080p.BluRay.x264.mkv",
	"テスト番組.S02E03.720p.HDTV.mkv",

	"Test Show Gamma/Season 01/Test Show Gamma S01E01 1080p WEB-DL.mkv",
	"Test Show Gamma/Season 01/Test Show Gamma S01E02 1080p WEB-DL.mkv",
	"Test Show Delta/Season 03/Episode 04 - Test Show Delta 1080p HDTV.mkv",
	"Test Anthology/Season 02/Test Anthology S02E05 720p WEB-DL.mkv",

	"www.TestIndexer.com.mkv",
	"Test.Feature.Sigma.2016.1080p.BluRay.x264-GROUP1.mkv",
	"Test.Feature.Sigma.2016.1080p.BluRay.x264-GROUP2.mkv",
	"Test.Feature.Beta.Two.2023.1080p.WEB-DL.DD5.1.H.264-TESTGRP.mkv",
	"Test.Documentary.Series.S01E01.Chapter.One.1080p.WEBRip.x264.mkv",
	"Test.Documentary.Series.S01E02.Chapter.Two.1080p.WEBRip.x264.mkv",
	"Test.Miniseries.Echo.S01E01.1080p.AMZN.WEB-DL.DDP5.1.H.264-FLUX.mkv",
	"Test.Miniseries.Echo.S01E02.1080p.AMZN.WEB-DL.DDP5.1.H.264-FLUX.mkv",
}

func BenchmarkParseMedia(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matcher.ParseMedia(benchNames[i%len(benchNames)])
	}
}

func BenchmarkMatchTorrentFiles(b *testing.B) {
	files := make([]torbox.TorrentFile, 24)
	for i := 0; i < 24; i++ {
		files[i] = torbox.TorrentFile{
			ID:   i + 1,
			Name: fmt.Sprintf("Test.Bench.Series.S01E%02d.Episode.Title.1080p.WEB-DL.DDP5.1.H.264-FLUX.mkv", i+1),
			Size: 1 << 30,
		}
	}

	m := matcher.NewMatcher(nil, nil, nil)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.MatchTorrentFiles(files)
	}
}
