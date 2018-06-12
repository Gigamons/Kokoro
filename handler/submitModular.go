package handler

import (
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Gigamons/Kokoro/calculate"
	"github.com/Gigamons/Kokoro/helper"
	oppai "github.com/Gigamons/oppai5"

	"github.com/Gigamons/common/consts"
	"github.com/Gigamons/common/helpers"

	"github.com/Gigamons/common/tools/usertools"

	"github.com/Gigamons/common/logger"
	"github.com/celso-wo/rijndael256"
)

type scoredata struct {
	FileMD5        string
	Username       string
	ScoreMD5       string // Unknown.
	Count300       int
	Count100       int
	Count50        int
	CountGeki      int
	CountKatu      int
	CountMiss      int
	Score          int
	MaxCombo       int
	FC             bool
	ArchivedLetter string
	Mods           int
	Pass           bool
	PlayMode       int
	Date           time.Time
	RawVersion     string
	BadFlags       int
}

func parseCryptedScoredata(score string, iv string, key string) (*scoredata, error) {
	Encoding := base64.StdEncoding
	ScoreENC, err := Encoding.DecodeString(score)
	if err != nil {
		return nil, err
	}
	IV, err := Encoding.DecodeString(iv)
	if err != nil {
		return nil, err
	}

	if key != "" {
		key = "osu!-scoreburgr---------" + key
	} else {
		key = "h89f2-890h2h89b34g-h80g134n90133"
	}

	block, err := rijndael256.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}

	Decipher := cipher.NewCBCDecrypter(block, IV)

	Decipher.CryptBlocks(ScoreENC, ScoreENC)

	data := strings.Split(string(ScoreENC), ":")
	count300, err := strconv.Atoi(data[3])
	if err != nil {
		return nil, err
	}
	count100, err := strconv.Atoi(data[4])
	if err != nil {
		return nil, err
	}
	count50, err := strconv.Atoi(data[5])
	if err != nil {
		return nil, err
	}
	countGeki, err := strconv.Atoi(data[6])
	if err != nil {
		return nil, err
	}
	countKatu, err := strconv.Atoi(data[7])
	if err != nil {
		return nil, err
	}
	countMiss, err := strconv.Atoi(data[8])
	if err != nil {
		return nil, err
	}
	scr, err := strconv.Atoi(data[9])
	if err != nil {
		return nil, err
	}
	maxCombo, err := strconv.Atoi(data[10])
	if err != nil {
		return nil, err
	}
	mods, err := strconv.Atoi(data[13])
	if err != nil {
		return nil, err
	}
	playMode, err := strconv.Atoi(data[15])
	if err != nil {
		return nil, err
	}

	ScoreData := &scoredata{
		FileMD5:  data[0],
		Username: strings.TrimSpace(data[1]),
		ScoreMD5: hex.EncodeToString(func() []byte {
			md := md5.New()
			fmt.Fprintf(md, "%v%v%s%v", count300+count100, count50, data[0], scr)
			return md.Sum(nil)
		}()),
		Count300:       count300,
		Count100:       count100,
		Count50:        count50,
		CountGeki:      countGeki,
		CountKatu:      countKatu,
		CountMiss:      countMiss,
		Score:          scr,
		MaxCombo:       maxCombo,
		FC:             data[11] == "True",
		ArchivedLetter: data[12],
		Mods:           mods,
		Pass:           data[14] == "True",
		PlayMode:       playMode,
		Date:           time.Now(),
		RawVersion:     data[17],
		BadFlags:       len(data[17]) - len(strings.TrimSpace(data[17])) & ^4,
	}
	return ScoreData, nil
}

func POSTSubmitModular(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(0)
	if err != nil {
		w.WriteHeader(500)
		logger.Errorln(err)
		return
	}

	ScoreData, err := parseCryptedScoredata(r.FormValue("score"), r.FormValue("iv"), r.FormValue("osuver"))
	if err != nil {
		return
	}
	Password := r.FormValue("pass")
	User := usertools.GetUser(usertools.GetUserID(ScoreData.Username))

	if User == nil {
		fmt.Fprint(w, "error: unkown")
		return
	} else if User.CheckPassword(Password) {
		Beatmap := helper.GetBeatmapofDBHash(ScoreData.FileMD5)
		if Beatmap == nil {
			return
		}
		IsRanked := Beatmap.IsRanked()
		IsLoved := Beatmap.IsLoved()

		if (IsRanked || IsLoved) && ScoreData.Pass {
			increaseTotalScore(User, ScoreData.Score, ScoreData.PlayMode)
			increasePlaycount(User, ScoreData.PlayMode)
			if IsRanked {
				increaseRankedScore(User, ScoreData.Score, ScoreData.PlayMode)
			}
			increaseCount300(User, ScoreData.Count300, ScoreData.PlayMode)
			increaseCount100(User, ScoreData.Count100, ScoreData.PlayMode)
			increaseCount50(User, ScoreData.Count50, ScoreData.PlayMode)
			increaseCountMiss(User, ScoreData.CountMiss, ScoreData.PlayMode)
			DownloadedMap, err := helpers.DownloadBeatmapbyName(strconv.Itoa(Beatmap.BeatmapID))
			if err != nil {
				w.WriteHeader(408)
				logger.Errorln(err)
				return
			}
			GivePP := true
			if DownloadedMap == "" {
				GivePP = false
			}
			var DaPP float64
			if GivePP && ScoreData.PlayMode == consts.STD {
				f, err := os.Open(DownloadedMap)
				if err != nil {
					w.WriteHeader(408)
					logger.Errorln(err)
					return
				}
				defer f.Close()
				ParsedMap := oppai.Parse(f)
				PP := oppai.PPInfo(ParsedMap, &oppai.Parameters{
					Combo:  uint16(ScoreData.MaxCombo),
					Misses: uint16(ScoreData.CountMiss),
					Mods:   uint32(ScoreData.Mods),
					N300:   uint16(ScoreData.Count300),
					N100:   uint16(ScoreData.Count100),
					N50:    uint16(ScoreData.Count50),
				})
				DaPP = PP.PP.Total
			}
			Replay, _, err := r.FormFile("score")
			if err != nil {
				w.WriteHeader(408)
				logger.Errorln(err)
				return
			}
			defer Replay.Close()
			replay, err := ioutil.ReadAll(Replay)
			if err != nil {
				w.WriteHeader(408)
				logger.Errorln(err)
				return
			}
			h := md5.New()
			h.Write(replay)
			ReplayMD5 := hex.EncodeToString(h.Sum(nil))
			err = insertScore(User, Beatmap, ScoreData, ReplayMD5, DaPP)
			if err != nil {
				w.WriteHeader(408)
				logger.Errorln(err)
				return
			}
			err = insertReplay(ScoreData, ReplayMD5, replay)
			if err != nil {
				w.WriteHeader(408)
				deleteScore(ScoreData)
				logger.Errorln(err)
				return
			}
			//LeaderboardOLD := usertools.GetLeaderboard(*User, int8(ScoreData.PlayMode))
			calculate.CalculateUser(int(User.ID), ScoreData.Mods&consts.ModsRX != 0 || ScoreData.Mods&consts.ModsRX2 != 0, int8(ScoreData.PlayMode))
			//LeaderboardNEW := usertools.GetLeaderboard(*User, int8(ScoreData.PlayMode))
			outputstring := ""
			/* --- Beatmap --- */
			outputstring += "beatmapid:" + strconv.Itoa(Beatmap.BeatmapID) + "|"
			outputstring += "beatmapSetId:" + strconv.Itoa(Beatmap.SetID) + "|"
			outputstring += "beatmapSetId:" + strconv.Itoa(Beatmap.SetID) + "|"
			outputstring += "beatmapPlaycount:0|"
			outputstring += "beatmapPasscount:0|"
			outputstring += "approvedDate:" + Beatmap.RankedDate + "\n"
			/* --- Charts --- */
			outputstring += "chartId:overall|"
			if ScoreData.Mods&consts.ModsRX != 0 || ScoreData.Mods&consts.ModsRX2 != 0 {
				outputstring += "chartName:Relax Ranking|"
			} else {
				outputstring += "chartName:Overall Ranking|"
			}
			outputstring += "chartEndDate:|"
			/* --- Ranking --- */
			outputstring += "beatmapRankingBefore:0|"
			outputstring += "beatmapRankingAfter:0|"
			outputstring += "rankedScoreBefore:0|"
			outputstring += "rankedScoreAfter:0|"
			outputstring += "totalScoreBefore:0|"
			outputstring += "totalScoreAfter:0|"
			outputstring += "playCountBefore:0|"
			outputstring += "accuracyBefore:0|"
			outputstring += "accuracyAfter:0|"
			outputstring += "rankBefore:0|"
			outputstring += "rankAfter:0|"
			outputstring += "toNextRank:0|"
			outputstring += "toNextRankUser:0|"
			/* --- Stats --- */
			outputstring += "achievements:|"
			outputstring += "achievements-new:|"
			outputstring += "onlineScoreId:0|"
			outputstring += "\n"

			logger.Debugln(outputstring)
			fmt.Fprint(w, outputstring)
		} else {
			increaseTotalScore(User, ScoreData.Score, ScoreData.PlayMode)
			increasePlaycount(User, ScoreData.PlayMode)
			fmt.Fprintf(w, "ok")
		}
	} else {
		fmt.Fprint(w, "error: pass")
	}
}

func insertScore(u *consts.User, bm *helper.DBBeatmap, sd *scoredata, ReplayMD5 string, PP float64) error {
	logger.Debugln(sd.Count300, sd.Count100, sd.Count50, sd.CountMiss, sd.CountGeki, sd.CountKatu, sd.PlayMode)
	logger.Debugln(helpers.CalculateAccuracy(
		int64(sd.Count300), int64(sd.Count100),
		int64(sd.Count50), int64(sd.CountMiss),
		int64(sd.CountGeki), int64(sd.CountKatu),
		int8(sd.PlayMode),
	))
	_, err := helpers.DB.Exec(`
			INSERT INTO scores
			(
				UserID,
				FileMD5,
				ScoreMD5,
				ReplayMD5,
				Score,
				MaxCombo,
				PlayMode,
				Mods,
				Count300,
				Count100,
				Count50,
				CountMiss,
				CountGeki,
				CountKatu,
				Date,
				Accuracy,
				PeppyPoints
			)
			Values
			(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		`, u.ID, sd.FileMD5, sd.ScoreMD5, ReplayMD5, sd.Score, sd.MaxCombo, sd.PlayMode,
		sd.Mods, sd.Count300, sd.Count100, sd.Count50, sd.CountMiss, sd.CountGeki, sd.CountKatu,
		sd.Date, helpers.CalculateAccuracy(
			int64(sd.Count300), int64(sd.Count100),
			int64(sd.Count50), int64(sd.CountMiss),
			int64(sd.CountGeki), int64(sd.CountKatu),
			int8(sd.PlayMode),
		), PP,
	)
	return err
}

func insertReplay(sd *scoredata, ReplayMD5 string, Replay []byte) error {
	_, err := helpers.DB.Exec(
		`
			INSERT INTO replays
			(
				ScoreMD5,
				ReplayMD5,
				Replay
			)
			Values
			(?,?,?)
		`, sd.ScoreMD5, ReplayMD5, Replay,
	)
	return err
}

func deleteScore(sd *scoredata) {
	helpers.DB.Exec("DELETE FROM scores WHERE ScoreMD5 = ?", sd.ScoreMD5)
}

func increasePlaycount(u *consts.User, playMode int) {
	helpers.DB.Exec("UPDATE leaderboard SET playcount_"+consts.ToPlaymodeString(int8(playMode))+" = playcount_"+consts.ToPlaymodeString(int8(playMode))+" + 1 WHERE id = ?", u.ID)
}

func increaseCount300(u *consts.User, Count300 int, playMode int) {
	helpers.DB.Exec("UPDATE leaderboard SET count_300_"+consts.ToPlaymodeString(int8(playMode))+" = count_300_"+consts.ToPlaymodeString(int8(playMode))+" + ? WHERE id = ?", Count300, u.ID)
}

func increaseCount100(u *consts.User, Count100 int, playMode int) {
	helpers.DB.Exec("UPDATE leaderboard SET count_100_"+consts.ToPlaymodeString(int8(playMode))+" = count_100_"+consts.ToPlaymodeString(int8(playMode))+" + ? WHERE id = ?", Count100, u.ID)
}

func increaseCount50(u *consts.User, Count50 int, playMode int) {
	helpers.DB.Exec("UPDATE leaderboard SET count_50_"+consts.ToPlaymodeString(int8(playMode))+" = count_50_"+consts.ToPlaymodeString(int8(playMode))+" + ? WHERE id = ?", Count50, u.ID)
}

func increaseCountMiss(u *consts.User, CountMiss int, playMode int) {
	helpers.DB.Exec("UPDATE leaderboard SET count_miss_"+consts.ToPlaymodeString(int8(playMode))+" = count_miss_"+consts.ToPlaymodeString(int8(playMode))+" + ? WHERE id = ?", CountMiss, u.ID)
}

func increaseTotalScore(u *consts.User, Score int, playMode int) {
	helpers.DB.Exec("UPDATE leaderboard SET totalscore_"+consts.ToPlaymodeString(int8(playMode))+" = totalscore_"+consts.ToPlaymodeString(int8(playMode))+" + ? WHERE id = ?", Score, u.ID)
}

func increaseRankedScore(u *consts.User, Score int, playMode int) {
	helpers.DB.Exec("UPDATE leaderboard SET rankedscore_"+consts.ToPlaymodeString(int8(playMode))+" = rankedscore_"+consts.ToPlaymodeString(int8(playMode))+" + ? WHERE id = ?", Score, u.ID)
}
