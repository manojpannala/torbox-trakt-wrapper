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
			name:     "test_runner_2049_reverse_year",
			filename: "Test.Runner.2049.2017.1080p.BluRay.x264.DDP5.1.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Runner 2049",
				Type:       matcher.MediaTypeMovie,
				Year:       2017,
				Resolution: "1080p",
				Source:     "bluray",
				Codec:      "x264",
				Audio:      "ddp51",
			},
			displayTitle: "Test Runner 2049 (2017)",
		},
		{
			name:     "test_2001_space_odyssey_reverse_year",
			filename: "2001.A.Test.Space.Odyssey.1968.2160p.UHD.BluRay.x265.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "2001 A Test Space Odyssey",
				Type:       matcher.MediaTypeMovie,
				Year:       1968,
				Resolution: "2160p",
				Source:     "bluray",
				Codec:      "x265",
			},
			displayTitle: "2001 A Test Space Odyssey (1968)",
		},
		{
			name:     "test_1917_year_as_title",
			filename: "1917.2019.1080p.BluRay.x264.DTS.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "1917",
				Type:       matcher.MediaTypeMovie,
				Year:       2019,
				Resolution: "1080p",
				Source:     "bluray",
				Codec:      "x264",
				Audio:      "dts",
			},
			displayTitle: "1917 (2019)",
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
			filename: "Test.Thrones.Show.Epsilon.S01E01-E02.1080p.BluRay.x265.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Thrones Show Epsilon",
				Type:       matcher.MediaTypeEpisode,
				Season:     1,
				Episode:    1,
				EpisodeEnd: 2,
				Resolution: "1080p",
				Source:     "bluray",
				Codec:      "x265",
			},
			displayTitle: "Test Thrones Show Epsilon S01E01-E02",
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
			name:     "test_mystery_eta_alt_season_ep",
			filename: "Test.Island.Mystery.Eta.01x05.Episode.Title.720p.HDTV.x264.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Island Mystery Eta",
				Type:       matcher.MediaTypeEpisode,
				Season:     1,
				Episode:    5,
				Resolution: "720p",
				Source:     "hdtv",
				Codec:      "x264",
			},
			displayTitle: "Test Island Mystery Eta S01E05",
		},
		{
			name:     "test_bounty_hunter_explicit_season_ep",
			filename: "The.Test.Bounty.Hunter.Season.1.Episode.8.2160p.WEB-DL.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "The Test Bounty Hunter",
				Type:       matcher.MediaTypeEpisode,
				Season:     1,
				Episode:    8,
				Resolution: "2160p",
				Source:     "webdl",
			},
			displayTitle: "The Test Bounty Hunter S01E08",
		},
		{
			name:     "test_sorcery_anime_bracketed",
			filename: "[Test-Group] Test Sorcery Anime - 24 [1080p][HEVC][AAC].mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "Test Sorcery Anime",
				Type:         matcher.MediaTypeEpisode,
				Season:       1,
				Episode:      24,
				Resolution:   "1080p",
				Codec:        "hevc",
				Audio:        "aac",
				ReleaseGroup: "Test-Group",
			},
			displayTitle: "Test Sorcery Anime S01E24",
		},
		{
			name:     "test_journey_anime_hash",
			filename: "[Test-Group] Test Journey Story - 12 (1080p) [9A71B52F].mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "Test Journey Story",
				Type:         matcher.MediaTypeEpisode,
				Season:       1,
				Episode:      12,
				Resolution:   "1080p",
				ReleaseGroup: "Test-Group",
			},
			displayTitle: "Test Journey Story S01E12",
		},
		{
			name:     "test_titan_anime_flat",
			filename: "Test Titan Warrior - 75 [1080p].mp4",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Titan Warrior",
				Type:       matcher.MediaTypeEpisode,
				Season:     1,
				Episode:    75,
				Resolution: "1080p",
			},
			displayTitle: "Test Titan Warrior S01E75",
		},
		{
			name:     "test_fellowship_extended_truehd_atmos",
			filename: "The.Test.Ring.Fellowship.2001.EXTENDED.2160p.UHD.BluRay.x265.TrueHD.7.1.Atmos-FLUX.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "The Test Ring Fellowship",
				Type:         matcher.MediaTypeMovie,
				Year:         2001,
				Resolution:   "2160p",
				Source:       "bluray",
				Codec:        "x265",
				Audio:        "truehdatmos",
				ReleaseGroup: "FLUX",
			},
			displayTitle: "The Test Ring Fellowship (2001)",
		},
		{
			name:     "test_hero_multiverse_aac51",
			filename: "Test-Hero.Across.the.Multiverse.2023.1080p.WEBRip.x264.AAC5.1.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test-Hero Across the Multiverse",
				Type:       matcher.MediaTypeMovie,
				Year:       2023,
				Resolution: "1080p",
				Source:     "webrip",
				Codec:      "x264",
				Audio:      "aac51",
			},
			displayTitle: "Test-Hero Across the Multiverse (2023)",
		},
		{
			name:     "test_space_voyage_underscores",
			filename: "Test_Space_Voyage_2014_1080p_BluRay_x264_DTS.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Space Voyage",
				Type:       matcher.MediaTypeMovie,
				Year:       2014,
				Resolution: "1080p",
				Source:     "bluray",
				Codec:      "x264",
				Audio:      "dts",
			},
			displayTitle: "Test Space Voyage (2014)",
		},
		{
			name:     "test_jet_pilot_iso_image",
			filename: "Test.Jet.Pilot.2022.2160p.UHD.BluRay.iso",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Jet Pilot",
				Type:       matcher.MediaTypeMovie,
				Year:       2022,
				Resolution: "2160p",
				Source:     "bluray",
			},
			displayTitle: "Test Jet Pilot (2022)",
		},
		{
			name:     "test_everything_omniverse_av1_flac",
			filename: "Test.Everything.Omniverse.2022.1080p.AV1.FLAC.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Everything Omniverse",
				Type:       matcher.MediaTypeMovie,
				Year:       2022,
				Resolution: "1080p",
				Codec:      "av1",
				Audio:      "flac",
			},
			displayTitle: "Test Everything Omniverse (2022)",
		},
		{
			name:     "test_brawler_guild_xvid_ac3",
			filename: "Test.Brawler.Guild.1999.DVDRip.XviD.AC3-EVO.avi",
			expected: matcher.ParsedMedia{
				CleanTitle:   "Test Brawler Guild",
				Type:         matcher.MediaTypeMovie,
				Year:         1999,
				Source:       "dvdrip",
				Codec:        "xvid",
				Audio:        "ac3",
				ReleaseGroup: "EVO",
			},
			displayTitle: "Test Brawler Guild (1999)",
		},
		{
			name:     "test_kitchen_full_path",
			filename: "/downloads/complete/The.Test.Kitchen.S02E06.Episode.1080p.HULU.WEB-DL.DDP5.1.H.264-FLUX.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "The Test Kitchen",
				Type:         matcher.MediaTypeEpisode,
				Season:       2,
				Episode:      6,
				Resolution:   "1080p",
				Source:       "webdl",
				Codec:        "h264",
				Audio:        "ddp51",
				ReleaseGroup: "FLUX",
			},
			displayTitle: "The Test Kitchen S02E06",
		},
		{
			name:     "test_dragon_dynasty_4k",
			filename: "Test.Dragon.Dynasty.S01E10.Episode.Title.2160p.UHD.HDR.H.265.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Dragon Dynasty",
				Type:       matcher.MediaTypeEpisode,
				Season:     1,
				Episode:    10,
				Resolution: "2160p",
				Codec:      "h265",
				HDR:        "hdr",
			},
			displayTitle: "Test Dragon Dynasty S01E10",
		},
		{
			name:     "test_infection_season_ep",
			filename: "The.Test.Infection.S01E03.Episode.Title.1080p.MAX.WEBRip.x265.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "The Test Infection",
				Type:       matcher.MediaTypeEpisode,
				Season:     1,
				Episode:    3,
				Resolution: "1080p",
				Source:     "webrip",
				Codec:      "x265",
			},
			displayTitle: "The Test Infection S01E03",
		},
		{
			name:     "test_attorney_chronicles_720p",
			filename: "Test.Attorney.Chronicles.S06E13.Episode.Title.720p.HDTV.x264.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Attorney Chronicles",
				Type:       matcher.MediaTypeEpisode,
				Season:     6,
				Episode:    13,
				Resolution: "720p",
				Source:     "hdtv",
				Codec:      "x264",
			},
			displayTitle: "Test Attorney Chronicles S06E13",
		},
		{
			name:     "test_saw_fiend_anime_flat",
			filename: "Test Saw Fiend - 01 [1080p].mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Saw Fiend",
				Type:       matcher.MediaTypeEpisode,
				Season:     1,
				Episode:    1,
				Resolution: "1080p",
			},
			displayTitle: "Test Saw Fiend S01E01",
		},
		{
			name:     "test_pink_doll_dovi",
			filename: "Test.Pink.Doll.2023.2160p.WEB-DL.DDP5.1.Atmos.DoVi.HEVC-CMRG.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "Test Pink Doll",
				Type:         matcher.MediaTypeMovie,
				Year:         2023,
				Resolution:   "2160p",
				Source:       "webdl",
				Codec:        "hevc",
				Audio:        "ddp51",
				HDR:          "dovi",
				ReleaseGroup: "CMRG",
			},
			displayTitle: "Test Pink Doll (2023)",
		},
		{
			name:     "test_virtual_wars_dtshdma",
			filename: "Test.Virtual.Wars.2009.1080p.BluRay.x264.DTS-HD.MA.5.1.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle: "Test Virtual Wars",
				Type:       matcher.MediaTypeMovie,
				Year:       2009,
				Resolution: "1080p",
				Source:     "bluray",
				Codec:      "x264",
				Audio:      "dtshdma",
			},
			displayTitle: "Test Virtual Wars (2009)",
		},
		{
			name:     "test_jurors_classic_1957",
			filename: "12.Test.Jurors.1957.1080p.Criterion.BluRay.x264.FLAC-EA.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "12 Test Jurors",
				Type:         matcher.MediaTypeMovie,
				Year:         1957,
				Resolution:   "1080p",
				Source:       "bluray",
				Codec:        "x264",
				Audio:        "flac",
				ReleaseGroup: "EA",
			},
			displayTitle: "12 Test Jurors (1957)",
		},
		{
			name:     "test_monster_zero_truehd_atmos",
			filename: "Test.Monster.Zero.2023.2160p.UHD.BluRay.TrueHD.7.1.Atmos.DV.HDR10.HEVC-FLUX.mkv",
			expected: matcher.ParsedMedia{
				CleanTitle:   "Test Monster Zero",
				Type:         matcher.MediaTypeMovie,
				Year:         2023,
				Resolution:   "2160p",
				Source:       "bluray",
				Codec:        "hevc",
				Audio:        "truehdatmos",
				ReleaseGroup: "FLUX",
			},
			displayTitle: "Test Monster Zero (2023)",
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
