package matcher_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
)

func TestParseMedia_ExhaustiveMatrix(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		expected     matcher.ParsedMedia
		displayTitle string
	}{
		{
			name:     "test_movie_alpha_2160p_remux",
			filename: "Test.Movie.Alpha.2023.2160p.UHD.Remux.HEVC.TrueHD.Atmos-FLUX.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "Test Movie Alpha",
				Type:         matcher.MediaTypeMovie,
				Year:         2023,
				Resolution:   "2160p",
				Source:       "remux",
				Codec:        "hevc",
				Audio:        "truehdatmos",
				ReleaseGroup: "FLUX",
			},
			displayTitle: "Test Movie Alpha (2023)",
		},
		{
			name:     "test_film_beta_1080p_bluray",
			filename: "The.Test.Film.Beta.2008.1080p.BluRay.x264.DTS-WiKi.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "The Test Film Beta",
				Type:         matcher.MediaTypeMovie,
				Year:         2008,
				Resolution:   "1080p",
				Source:       "bluray",
				Codec:        "x264",
				Audio:        "dts",
				ReleaseGroup: "WiKi",
			},
			displayTitle: "The Test Film Beta (2008)",
		},
		{
			name:     "test_sequel_gamma_dv_hdr10plus",
			filename: "Test.Sequel.Gamma.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR10+.H.265-FLUX.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "Test Sequel Gamma Part Two",
				Type:         matcher.MediaTypeMovie,
				Year:         2024,
				Resolution:   "2160p",
				Source:       "webdl",
				Codec:        "h265",
				Audio:        "ddp51",
				HDR:          "dv",
				ReleaseGroup: "FLUX",
			},
			displayTitle: "Test Sequel Gamma Part Two (2024)",
		},
		{
			name:     "test_trailing_number_reverse_year",
			filename: "Test.Colony.Theta.2049.2017.1080p.BluRay.x264.DDP5.1.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Colony Theta 2049",
				Type:       matcher.MediaTypeMovie,
				Year:       2017,
				Resolution: "1080p",
				Source:     "bluray",
				Codec:      "x264",
				Audio:      "ddp51",
			},
			displayTitle: "Test Colony Theta 2049 (2017)",
		},
		{
			name:     "test_leading_number_reverse_year",
			filename: "2001.A.Test.Feature.Iota.1968.2160p.UHD.BluRay.x265.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "2001 A Test Feature Iota",
				Type:       matcher.MediaTypeMovie,
				Year:       1968,
				Resolution: "2160p",
				Source:     "bluray",
				Codec:      "x265",
			},
			displayTitle: "2001 A Test Feature Iota (1968)",
		},
		{
			name:     "test_bare_year_as_title",
			filename: "1863.2019.1080p.BluRay.x264.DTS.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "1863",
				Type:       matcher.MediaTypeMovie,
				Year:       2019,
				Resolution: "1080p",
				Source:     "bluray",
				Codec:      "x264",
				Audio:      "dts",
			},
			displayTitle: "1863 (2019)",
		},
		{
			name:     "test_future_2077_no_year",
			filename: "Test.Future.2077.No.Year.1080p.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Future 2077 No Year",
				Type:       matcher.MediaTypeMovie,
				Resolution: "1080p",
			},
			displayTitle: "Test Future 2077 No Year",
		},
		{
			name:     "test_crime_series_s01e05",
			filename: "Test.Crime.Series.Delta.S01E05.Episode.Name.1080p.BluRay.x264-DEMAND.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "Test Crime Series Delta",
				Type:         matcher.MediaTypeEpisode,
				Season:       1,
				Episode:      5,
				Resolution:   "1080p",
				Source:       "bluray",
				Codec:        "x264",
				ReleaseGroup: "DEMAND",
			},
			displayTitle: "Test Crime Series Delta S01E05",
		},
		{
			name:     "test_show_epsilon_multiepisodes",
			filename: "Test.Show.Epsilon.S01E01-E02.1080p.BluRay.x265.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Show Epsilon",
				Type:       matcher.MediaTypeEpisode,
				Season:     1,
				Episode:    1,
				EpisodeEnd: 2,
				Resolution: "1080p",
				Source:     "bluray",
				Codec:      "x265",
			},
			displayTitle: "Test Show Epsilon S01E01-E02",
		},
		{
			name:     "test_drama_zeta_multiepisodes_hyphen",
			filename: "Test.Corporate.Drama.Zeta.S02E01-03.1080p.WEB-DL.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Corporate Drama Zeta",
				Type:       matcher.MediaTypeEpisode,
				Season:     2,
				Episode:    1,
				EpisodeEnd: 3,
				Resolution: "1080p",
				Source:     "webdl",
			},
			displayTitle: "Test Corporate Drama Zeta S02E01-E03",
		},
		{
			name:     "test_series_eta_alt_season_ep",
			filename: "Test.Series.Eta.01x05.Episode.Title.720p.HDTV.x264.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Series Eta",
				Type:       matcher.MediaTypeEpisode,
				Season:     1,
				Episode:    5,
				Resolution: "720p",
				Source:     "hdtv",
				Codec:      "x264",
			},
			displayTitle: "Test Series Eta S01E05",
		},
		{
			name:     "test_series_theta_explicit_season_ep",
			filename: "The.Test.Series.Theta.Season.1.Episode.8.2160p.WEB-DL.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "The Test Series Theta",
				Type:       matcher.MediaTypeEpisode,
				Season:     1,
				Episode:    8,
				Resolution: "2160p",
				Source:     "webdl",
			},
			displayTitle: "The Test Series Theta S01E08",
		},
		{
			name:     "test_anime_omicron_bracketed",
			filename: "[Test-Group] Test Anime Omicron - 24 [1080p][HEVC][AAC].mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "Test Anime Omicron",
				Type:         matcher.MediaTypeEpisode,
				Season:       1,
				Episode:      24,
				Resolution:   "1080p",
				Codec:        "hevc",
				Audio:        "aac",
				ReleaseGroup: "Test-Group",
			},
			displayTitle: "Test Anime Omicron S01E24",
		},
		{
			name:     "test_anime_pi_hash",
			filename: "[Test-Group] Test Anime Pi - 12 (1080p) [9A71B52F].mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "Test Anime Pi",
				Type:         matcher.MediaTypeEpisode,
				Season:       1,
				Episode:      12,
				Resolution:   "1080p",
				ReleaseGroup: "Test-Group",
			},
			displayTitle: "Test Anime Pi S01E12",
		},
		{
			name:     "test_anime_mu_flat",
			filename: "Test Anime Mu - 75 [1080p].mp4",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Anime Mu",
				Type:       matcher.MediaTypeEpisode,
				Season:     1,
				Episode:    75,
				Resolution: "1080p",
			},
			displayTitle: "Test Anime Mu S01E75",
		},
		{
			name:     "test_feature_kappa_extended_truehd_atmos",
			filename: "The.Test.Feature.Kappa.2001.EXTENDED.2160p.UHD.BluRay.x265.TrueHD.7.1.Atmos-FLUX.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "The Test Feature Kappa",
				Type:         matcher.MediaTypeMovie,
				Year:         2001,
				Resolution:   "2160p",
				Source:       "bluray",
				Codec:        "x265",
				Audio:        "truehdatmos",
				ReleaseGroup: "FLUX",
			},
			displayTitle: "The Test Feature Kappa (2001)",
		},
		{
			name:     "test_feature_xi_hyphenated_aac51",
			filename: "Test-Feature.Xi.Across.the.Line.2023.1080p.WEBRip.x264.AAC5.1.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test-Feature Xi Across the Line",
				Type:       matcher.MediaTypeMovie,
				Year:       2023,
				Resolution: "1080p",
				Source:     "webrip",
				Codec:      "x264",
				Audio:      "aac51",
			},
			displayTitle: "Test-Feature Xi Across the Line (2023)",
		},
		{
			name:     "test_feature_omicron_underscores",
			filename: "Test_Feature_Omicron_2014_1080p_BluRay_x264_DTS.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Feature Omicron",
				Type:       matcher.MediaTypeMovie,
				Year:       2014,
				Resolution: "1080p",
				Source:     "bluray",
				Codec:      "x264",
				Audio:      "dts",
			},
			displayTitle: "Test Feature Omicron (2014)",
		},
		{
			name:     "test_feature_pi_iso_image",
			filename: "Test.Feature.Pi.2022.2160p.UHD.BluRay.iso",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Feature Pi",
				Type:       matcher.MediaTypeMovie,
				Year:       2022,
				Resolution: "2160p",
				Source:     "bluray",
			},
			displayTitle: "Test Feature Pi (2022)",
		},
		{
			name:     "test_feature_rho_av1_flac",
			filename: "Test.Feature.Rho.2022.1080p.AV1.FLAC.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Feature Rho",
				Type:       matcher.MediaTypeMovie,
				Year:       2022,
				Resolution: "1080p",
				Codec:      "av1",
				Audio:      "flac",
			},
			displayTitle: "Test Feature Rho (2022)",
		},
		{
			name:     "test_feature_sigma_xvid_ac3",
			filename: "Test.Feature.Sigma.1999.DVDRip.XviD.AC3-EVO.avi",
			expected: matcher.ParsedMedia{
				CleanTitle:   "Test Feature Sigma",
				Type:         matcher.MediaTypeMovie,
				Year:         1999,
				Source:       "dvdrip",
				Codec:        "xvid",
				Audio:        "ac3",
				ReleaseGroup: "EVO",
			},
			displayTitle: "Test Feature Sigma (1999)",
		},
		{
			name:     "test_series_iota_full_path",
			filename: "/downloads/complete/The.Test.Series.Iota.S02E06.Episode.1080p.HULU.WEB-DL.DDP5.1.H.264-FLUX.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "The Test Series Iota",
				Type:         matcher.MediaTypeEpisode,
				Season:       2,
				Episode:      6,
				Resolution:   "1080p",
				Source:       "webdl",
				Codec:        "h264",
				Audio:        "ddp51",
				ReleaseGroup: "FLUX",
			},
			displayTitle: "The Test Series Iota S02E06",
		},
		{
			name:     "test_series_kappa_4k",
			filename: "Test.Series.Kappa.S01E10.Episode.Title.2160p.UHD.HDR.H.265.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Series Kappa",
				Type:       matcher.MediaTypeEpisode,
				Season:     1,
				Episode:    10,
				Resolution: "2160p",
				Codec:      "h265",
				HDR:        "hdr",
			},
			displayTitle: "Test Series Kappa S01E10",
		},
		{
			name:     "test_series_lambda_season_ep",
			filename: "The.Test.Series.Lambda.S01E03.Episode.Title.1080p.MAX.WEBRip.x265.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "The Test Series Lambda",
				Type:       matcher.MediaTypeEpisode,
				Season:     1,
				Episode:    3,
				Resolution: "1080p",
				Source:     "webrip",
				Codec:      "x265",
			},
			displayTitle: "The Test Series Lambda S01E03",
		},
		{
			name:     "test_series_xi_720p",
			filename: "Test.Series.Xi.S06E13.Episode.Title.720p.HDTV.x264.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Series Xi",
				Type:       matcher.MediaTypeEpisode,
				Season:     6,
				Episode:    13,
				Resolution: "720p",
				Source:     "hdtv",
				Codec:      "x264",
			},
			displayTitle: "Test Series Xi S06E13",
		},
		{
			name:     "test_anime_nu_flat",
			filename: "Test Anime Nu - 01 [1080p].mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Anime Nu",
				Type:       matcher.MediaTypeEpisode,
				Season:     1,
				Episode:    1,
				Resolution: "1080p",
			},
			displayTitle: "Test Anime Nu S01E01",
		},
		{
			name:     "test_feature_tau_dovi",
			filename: "Test.Feature.Tau.2023.2160p.WEB-DL.DDP5.1.Atmos.DoVi.HEVC-CMRG.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "Test Feature Tau",
				Type:         matcher.MediaTypeMovie,
				Year:         2023,
				Resolution:   "2160p",
				Source:       "webdl",
				Codec:        "hevc",
				Audio:        "ddp51",
				HDR:          "dovi",
				ReleaseGroup: "CMRG",
			},
			displayTitle: "Test Feature Tau (2023)",
		},
		{
			name:     "test_feature_upsilon_dtshdma",
			filename: "Test.Feature.Upsilon.2009.1080p.BluRay.x264.DTS-HD.MA.5.1.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Feature Upsilon",
				Type:       matcher.MediaTypeMovie,
				Year:       2009,
				Resolution: "1080p",
				Source:     "bluray",
				Codec:      "x264",
				Audio:      "dtshdma",
			},
			displayTitle: "Test Feature Upsilon (2009)",
		},
		{
			name:     "test_leading_small_number_1957",
			filename: "12.Test.Figures.Lambda.1957.1080p.Criterion.BluRay.x264.FLAC-EA.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "12 Test Figures Lambda",
				Type:         matcher.MediaTypeMovie,
				Year:         1957,
				Resolution:   "1080p",
				Source:       "bluray",
				Codec:        "x264",
				Audio:        "flac",
				ReleaseGroup: "EA",
			},
			displayTitle: "12 Test Figures Lambda (1957)",
		},
		{
			name:     "test_feature_phi_truehd_atmos",
			filename: "Test.Feature.Phi.2023.2160p.UHD.BluRay.TrueHD.7.1.Atmos.DV.HDR10.HEVC-FLUX.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "Test Feature Phi",
				Type:         matcher.MediaTypeMovie,
				Year:         2023,
				Resolution:   "2160p",
				Source:       "bluray",
				Codec:        "hevc",
				Audio:        "truehdatmos",
				ReleaseGroup: "FLUX",
			},
			displayTitle: "Test Feature Phi (2023)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed := matcher.ParseMedia(tc.filename)
			assert.Equal(t, tc.expected.CleanTitle, parsed.CleanTitle)
			assert.Equal(t, tc.expected.Type, parsed.Type)
			if tc.expected.Year > 0 {
				assert.Equal(t, tc.expected.Year, parsed.Year)
			}
			if tc.expected.Season > 0 {
				assert.Equal(t, tc.expected.Season, parsed.Season)
			}
			if tc.expected.Episode > 0 {
				assert.Equal(t, tc.expected.Episode, parsed.Episode)
			}
			if tc.expected.EpisodeEnd > 0 {
				assert.Equal(t, tc.expected.EpisodeEnd, parsed.EpisodeEnd)
			}
			if tc.expected.Resolution != "" {
				assert.Equal(t, tc.expected.Resolution, parsed.Resolution)
			}
			if tc.expected.Source != "" {
				assert.Equal(t, tc.expected.Source, parsed.Source)
			}
			if tc.expected.Codec != "" {
				assert.Equal(t, tc.expected.Codec, parsed.Codec)
			}
			if tc.expected.Audio != "" {
				assert.Equal(t, tc.expected.Audio, parsed.Audio)
			}
			if tc.expected.HDR != "" {
				assert.Equal(t, tc.expected.HDR, parsed.HDR)
			}
			if tc.expected.ReleaseGroup != "" {
				assert.Equal(t, tc.expected.ReleaseGroup, parsed.ReleaseGroup)
			}
			if tc.displayTitle != "" {
				assert.Equal(t, tc.displayTitle, parsed.DisplayTitle())
			}
		})
	}
}
