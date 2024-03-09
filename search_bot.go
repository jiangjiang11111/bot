package main
import (
	"bot/lib"
	"bot/model"
	"os"
	"strings"
	"bufio"
	"io"
	"bot/db"
	"strconv"
	"unicode/utf16"
    "github.com/redis/go-redis/v9"
	"encoding/base64"
	"time"
)

var g_sBotKey = "" 
var g_lSuperUser []string
var g_iPageCount = int64(20)
var g_bFreqCheck = false
var g_iKeyExpire = time.Minute * 5

func CheckSuperUser(user string)bool{
	for _, val := range g_lSuperUser{
		if val == user{
			return true
		}
	}
	return false
}

func GetTitleAndDesMap(keys []string)(map[string]string, map[string]string){
	var title_keys, des_keys []string
	for _, key := range keys {
		title_keys = append(title_keys, key + "_title")
		des_keys = append(des_keys, key + "_des")
	}
	mapTitle := make(map[string]string)
	mapDes := make(map[string]string)
	if len(keys) == 0{
		return mapTitle, mapDes
	}
	titles, err := db.BatchGetStruct(title_keys...)
	if err != nil {
		lib.XLogErr("batchget", err, title_keys)
		return mapTitle, mapDes
	}
	dess, err := db.BatchGetStruct(des_keys...)
	if err != nil {
		lib.XLogErr("batchget", err, des_keys)
		return mapTitle, mapDes
	}
	for _, val := range titles{
		t, _ := val.(string)
		items := strings.Split(t, "$$$")
		if len(items) != 2{
			lib.XLogErr("split", val, items)
			continue
		}
		mapTitle[items[1]] = items[0]
	}
	for _, val := range dess{
		t, _ := val.(string)
		items := strings.Split(t, "$$$")
		if len(items) != 2{
			lib.XLogErr("split", val, items)
			continue
		}
		mapDes[items[1]] = items[0]
	}
	return mapTitle, mapDes
}

func Index(username, title, des string)error{
	key_title := title + "$$$" + username
	key_des := des + "$$$" + username
	err := db.Set(key_title, "1")
	if err != nil {
		lib.XLogErr(err, username, title)
		return err
	}
	err = db.Set(key_des, "1")
	if err != nil {
		lib.XLogErr(err, username, des)
		return err
	}
	err = db.Set(username + "_title", key_title)
	if err != nil {
		lib.XLogErr(err, username, key_title)
		return err
	}
	err = db.Set(username + "_des", key_des)
	if err != nil {
		lib.XLogErr(err, username, key_des)
		return err
	}
	lib.XLogInfo("index", username, title, des)
	return nil
}

func DeleteIndex(username string)error{
	key_title, err := db.Get(username + "_title")
	if err != nil {
		lib.XLogErr("get key_title", err, username)
		return err
	}
	key_des, err := db.Get(username + "_des")
	if err != nil {
		lib.XLogErr("get key_des", err, username)
		return err
	}
	err = db.Del(key_title)
	if err != nil {
		lib.XLogErr("del", key_title)
		return err
	}
	err = db.Del(key_des)
	if err != nil {
		lib.XLogErr("del", key_des)
		return err
	}
	return nil
}

func UpdateIndex(username, title, des string)error{
	err := DeleteIndex(username)
	if err != nil {
		lib.XLogErr("DeleteIndex", username)
		return err
	}
	err = Index(username, title, des)
	if err != nil {
		lib.XLogErr("index", err, username, title, des)
		return err
	}
	return nil
}

func SearchIndex(keyword string, offset uint64, limit int64)([]string, uint64, error){
	var ids, keys []string
	cur := int64(0)
	cmd := "*" + keyword + "*"
	cursor := offset
	for cur < limit {
		diff := limit - cur
		tmp_keys, tmp_cursor, err := db.Search(cmd, cursor, diff)
		if err != nil {
			lib.XLogErr("Search", err, keyword, cursor, diff)
			return ids, tmp_cursor, err
		}
		keys = append(keys, tmp_keys...)
		cur += int64(len(tmp_keys))
		cursor = tmp_cursor
		if tmp_cursor == 0{
			break
		}
	}
	lib.XLogInfo(cmd, offset, limit, keys, cursor)
	for _, key := range keys{
		vals := strings.Split(key, "$$$")
		if len(vals) != 2 {
			lib.XLogErr("invalid id", key)
			if err := db.Expire(key, time.Second * 1); err != nil {
				lib.XLogErr("Expire", err, key)
			}
			continue
		}
		str_id := vals[len(vals)-1]
		ids = append(ids, str_id)
	}
	return ids, cursor, nil
}


func InitConfig(){
	if len(os.Args) != 2 {
		lib.XLogErr("invalid usage! example: ./program config_file")
		panic("invalid usage")
	}
	config_file := os.Args[1]
	config, err := os.Open(config_file)
	if err != nil {
		lib.XLogErr("open config fail", config_file)
		panic("load config error")
	}
	defer config.Close()

	br := bufio.NewReader(config)
	for {
		a, _, c := br.ReadLine()
		if c == io.EOF {
			break
		}
		line := string(a)
		idx := strings.Index(line, "=")
		if idx == -1 {
			lib.XLogErr("invalid config", line)
			break
		}
		lib.XLogInfo("config line", line)
		if line[0 : idx] == "key" {
			g_sBotKey = line[idx + 1:]
		}else if line[0 : idx] == "super_userid"{
			g_lSuperUser = strings.Split(line[idx + 1:], ",")
		}else if line[0 : idx] == "page_count"{
			if tmp, err := strconv.ParseInt(line[idx + 1:], 10, 64); err == nil{
				g_iPageCount = tmp
			}
		}else if line[0: idx] == "freq_check"{
			g_bFreqCheck = line[idx + 1:] == "1"
		} else if line[0: idx] == "key_expire"{
			if tmp, err := strconv.Atoi(line[idx + 1:]); err == nil{
				g_iKeyExpire = time.Second * time.Duration(tmp)
			}
		}
	}
}

func GetUTF16Len(content string)int{
	encodeContent := utf16.Encode([]rune(content))
	return len(encodeContent)
}

func LoadTopFeeds()map[string]string{
	mapUser2ShowTex := make(map[string]string)
	var feeds model.SearchTopFeeds
	if err := db.GetStruct("search_top_feeds", &feeds); err != nil {
		lib.XLogErr("GetStruct", err)
		return mapUser2ShowTex
	}
	for _, val := range feeds.Feeds{
		mapUser2ShowTex[val.UserName] = val.ShowText
	}
	return mapUser2ShowTex
}

func AddTopFeeds(username, showtext string)error{
	var feeds model.SearchTopFeeds
	err := db.GetStruct("search_top_feeds", &feeds)
	if err != nil && err != redis.Nil{
		lib.XLogErr("GetStruct", err)
		return err
	}
	new_feed := model.TopFeed{UserName:username, ShowText:showtext}
	feeds.Feeds = append(feeds.Feeds, new_feed)
	err = db.SetStruct("search_top_feeds", feeds)
	if err != nil {
		lib.XLogErr("SetStruct", err)
		return err
	}
	return nil
}

func DelTopFeeds(username string)error{
	var feeds model.SearchTopFeeds
	err := db.GetStruct("search_top_feeds", &feeds)
	if err != nil && err != redis.Nil{
		lib.XLogErr("GetStruct", err)
		return err
	}
	var new_feeds model.SearchTopFeeds
	for _, val := range feeds.Feeds{
		if val.UserName != username{
			new_feeds.Feeds = append(new_feeds.Feeds, val)
		}
	}
	err = db.SetStruct("search_top_feeds", new_feeds)
	if err != nil {
		lib.XLogErr("SetStruct", err, new_feeds)
		return err
	}
	return nil
}

func ProcessSearch(tb model.TBot, msg *model.Message){
	keyword := msg.Text
	lib.XLogInfo("keyword", keyword)
	// get all hit keys
	var total_keys []string
	var cursor uint64
	mapExist := make(map[string]bool)
	cursor = 0
	for {
		keys, tmp_cursor, err := SearchIndex(keyword, cursor, 100)
		if err != nil {
			lib.XLogErr("SearchIndex", err, keyword, cursor)
			return
		}
		cursor = tmp_cursor
		for _, val := range keys {
			if _, ok := mapExist[val]; ok {
				lib.XLogErr("repeated keys", val)
				continue
			}
			mapExist[val] = true
			total_keys = append(total_keys, val)
		}
		if tmp_cursor == 0 {
			break;
		}
	}
	result_key := base64.StdEncoding.EncodeToString([]byte(msg.From.UserName + "_" + keyword))

	hit, err := db.Exists(result_key)
	if err != nil {
		lib.XLogErr("Exists", err, result_key)
		return
	}
	if hit {
		if (g_bFreqCheck){
			lib.XLogErr("hit freq check", result_key)
			Warning(tb, msg.Chat.ID, msg.MessageID, msg.From.UserName, msg.From.ID, "频繁操作会被封号的!")
			return
		} else {
			if err := db.Expire(result_key, time.Second * 0); err != nil {
				lib.XLogErr("Expire", err, result_key)
			}
		}
	}
	if err := db.LPush(result_key, total_keys); err != nil {
		lib.XLogErr("LPush", err)
		return
	}
	if err := db.Expire(result_key, g_iKeyExpire); err != nil {
		lib.XLogErr("Expire", err, result_key)
	}
	lib.XLogInfo("result_key", result_key)

	var first_slice []string
	if len(total_keys) <= int(g_iPageCount){
		first_slice = total_keys
	} else {
		first_slice = total_keys[0 : g_iPageCount]
	}

	lib.XLogInfo(first_slice)
	mapTitle, _ := GetTitleAndDesMap(first_slice)
	send_config := model.SendMessageConfig{}
	send_config.ChatID = msg.Chat.ID
	send_config.ReplyParams = model.ReplyParameters{MessageID:msg.MessageID}
	msg_content := ""
	mapUser2ShowTex := LoadTopFeeds()
	for k, v := range mapUser2ShowTex{
		msg_content += "[置顶]"
		feed := model.MessageEntity{Type:"text_link"}
		feed.URL = "https://t.me/" + k
		feed.Offset = GetUTF16Len(msg_content)
		feed.Length = GetUTF16Len(v)
		send_config.Entities = append(send_config.Entities, feed)
		msg_content += v + "\n"
	}

	i := 0
	for k, v := range mapTitle {
		title := v
		url := model.MessageEntity{}
		url.Type = "text_link"
		url.URL = "https://t.me/" + k
		url.Offset = GetUTF16Len(msg_content)
		show_text := strconv.Itoa(i + 1) + ". " + title
		url.Length = GetUTF16Len(show_text)
		send_config.Entities = append(send_config.Entities, url)
		msg_content += show_text + "\n"
		i++
	}
	send_config.Text = msg_content
	send_config.LinkPreviewOption.IsDisable = true

	last_page := model.InlineKeyboardButton{Text:"上一页"}
	last_text := result_key + "$$-" + strconv.FormatInt(g_iPageCount, 10) + "$$last"
	last_page.CallbackData = &last_text
	next_page := model.InlineKeyboardButton{Text:"下一页"}
	next_text := result_key + "$$" + strconv.FormatInt(g_iPageCount, 10) + "$$next"
	next_page.CallbackData = &next_text
	var buttons []model.InlineKeyboardButton
	buttons = append(buttons, last_page, next_page)

	var markup model.InlineKeyboardMarkup
	markup.InlineKeyboard = append(markup.InlineKeyboard, buttons)

	send_config.ReplyMarkup = markup

	err = tb.Call(&send_config)
	if err != nil {
		lib.XLogErr(err, send_config)
	}
}

func GetSearchTextAndEntities(tb model.TBot, key string, cursor int64)(error, string, []model.MessageEntity){
	var entities []model.MessageEntity

	keys, err := db.LRange(key, cursor, cursor + g_iPageCount - 1)
	if err != nil {
		lib.XLogErr("SearchIndex", err, key, cursor)
		return err, "", entities
	}
	mapTitle, _ := GetTitleAndDesMap(keys)
	msg_content := ""
	mapUser2ShowTex := LoadTopFeeds()
	for k, v := range mapUser2ShowTex{
		msg_content += "[置顶]"
		feed := model.MessageEntity{Type:"text_link"}
		feed.URL = "https://t.me/" + k
		feed.Offset = GetUTF16Len(msg_content)
		feed.Length = GetUTF16Len(v)
		entities = append(entities, feed)
		msg_content += v + "\n"
	}

	i := cursor + 1
	for k, v := range mapTitle {
		title := v
		url := model.MessageEntity{}
		url.Type = "text_link"
		url.URL = "https://t.me/" + k
		url.Offset = GetUTF16Len(msg_content)
		show_text := strconv.FormatInt(i, 10) + ". " + title
		url.Length = GetUTF16Len(show_text)
		entities = append(entities, url)
		msg_content += show_text + "\n"
		i++
	}
	return nil, msg_content, entities
}

func AnswerCallback(tb model.TBot, queryid, text string){
	config := model.AnswerCallbackQueryConfig{CallbackID:queryid,Text:text}
	config.ShowAlert = true
	err := tb.Call(&config)
	if err != nil {
		lib.XLogErr(err)
		return
	}
}

func AddtoBlack(tb model.TBot, chatid, userid int64){
	config := model.RestrictChatMemberConfig{}
	config.ChatID = chatid
	config.UserID = userid

	perm := model.ChatPermissions{}
	perm.CanSendMessages = false
	config.Permissions = perm

	tb.Call(&config)
	lib.XLogInfo("addtoblack", chatid, userid)
}

func ProcessCallback(tb model.TBot, callback *model.CallbackQuery){
	if strings.HasPrefix(callback.Data, "addtoblack"){
		msg := callback.Message
		operator := callback.From.UserName
		if !CheckSuperUser(operator) {
			Warning(tb, msg.Chat.ID, msg.MessageID, callback.From.UserName, callback.From.ID, "别乱点!")
			return
		}
		items := strings.Split(callback.Data, "_")
		userid, err := strconv.ParseInt(items[1], 10, 64)
		if err != nil {
			lib.XLogErr("ParseInt", err, items)
			return
		}
		AddtoBlack(tb, msg.Chat.ID, userid)
		return
	}
	items := strings.Split(callback.Data, "$$")
	if len(items) != 3{
		lib.XLogErr("invalid callbackdata", callback.Data)
		return
	}
	config := model.EditMessageTextConfig{ChatID:callback.Message.Chat.ID, MessageID:callback.Message.MessageID}
	// 校验下操作者
	decoded, err := base64.StdEncoding.DecodeString(items[0])
	if err != nil {
		lib.XLogErr("decode error:", err)
		return
	}
	user_keyword := strings.Split(string(decoded), "_")
	if len(user_keyword) != 2{
		lib.XLogErr("invalid callbackdata", decoded)
		return
	}
	if callback.From.UserName != user_keyword[0]{
		AnswerCallback(tb, callback.ID, "不允许操作别人的搜索结果!")
		return
	}
	lib.XLogInfo("items", items)

	result_key := items[0]
	str_cursor := items[1]
	direction := items[2]
	lib.XLogInfo(result_key, str_cursor, direction)


	cursor, err := strconv.ParseInt(str_cursor, 10, 64)
	if err != nil {
		lib.XLogErr("ParseUint", str_cursor)
		return
	}

	if cursor < 0 {
		AnswerCallback(tb, callback.ID, "到头了!")
		return
	}

	total_count, err := db.LLen(result_key)
	if err != nil {
		lib.XLogErr("LLen", err, result_key)
		return
	}
	lib.XLogInfo("total_cont", total_count)
	if cursor >= total_count {
		AnswerCallback(tb, callback.ID, "到头了!")
		return
	}

	err, new_text, new_entities := GetSearchTextAndEntities(tb, result_key, cursor)
	if err != nil {
		lib.XLogErr("GetSearchTextAndEntities", result_key, cursor)
		return
	}
	config.Entities = new_entities
	config.Text = new_text
	// 上一页和下一页
	last_page := model.InlineKeyboardButton{Text:"上一页"}
	last_text := result_key + "$$" + strconv.FormatInt(cursor - g_iPageCount, 10) + "$$last"
	last_page.CallbackData = &last_text
	next_page := model.InlineKeyboardButton{Text:"下一页"}
	next_text := ""
	next_text = result_key + "$$" + strconv.FormatInt(cursor + g_iPageCount, 10) + "$$next"
	next_page.CallbackData = &next_text

	var btns []model.InlineKeyboardButton
	btns = append(btns, last_page, next_page)
	config.ReplyMarkup.InlineKeyboard = append(config.ReplyMarkup.InlineKeyboard, btns)
	config.LinkPreviewOption.IsDisable = true

	err = tb.Call(&config)
	if err != nil {
		lib.XLogErr("Call", err, config)
		return
	}
}

func HasBotCommand(msg *model.Message)bool{
	for _, et := range msg.Entities{
		if et.Type == "bot_command"{
			return true
		}
	}
	return false
}

func SendText(tb model.TBot, chatid int64, text string){
	config := model.SendMessageConfig{}
	config.ChatID = chatid
	config.Text = text
	tb.Call(&config)
}

func Warning(tb model.TBot, chatid int64, msgid int, username string, userid int64, text string){
	config := model.SendMessageConfig{}
	config.ChatID = chatid
	msg_content := text
	reply_param := model.ReplyParameters{}
	reply_param.ChatID = chatid
	reply_param.MessageID = msgid

	mention := model.MessageEntity{}
	mention.Type = "mention"
	mention.Offset = GetUTF16Len(msg_content)
	mention.Length = GetUTF16Len(username) + 1
	msg_content += "\n@" + username + ""

	addblack := model.InlineKeyboardButton{Text:"拉黑他"}
	addblack_text := "addtoblack_" + strconv.FormatInt(userid, 10)
	addblack.CallbackData = &addblack_text
	var buttons []model.InlineKeyboardButton
	buttons = append(buttons, addblack)

	var markup model.InlineKeyboardMarkup
	markup.InlineKeyboard = append(markup.InlineKeyboard, buttons)

	config.Text = msg_content
	config.ReplyParams = reply_param
	config.ReplyMarkup = markup
	tb.Call(&config)
}

func ProcessCommand(tb model.TBot, msg *model.Message){
	lib.XLogInfo("ProcessCommand", msg)
	from_user := msg.From.UserName
	hit := false
	for _, user := range g_lSuperUser{
		if user == from_user{
			hit = true
		}
	}
	if !hit{
		if msg.Chat.Type == "private"{
			SendText(tb, msg.Chat.ID, "command not for you!")
		}
		return
	}
	for _, et := range msg.Entities {
		if et.Type == "bot_command" {
			if len(msg.Text) < et.Offset + et.Length{
				lib.XLogErr("invalid command, maybe search", et, msg.Text)
				go ProcessSearch(tb, msg)
				continue
			}
			cmd := msg.Text[et.Offset + 1 : et.Offset + et.Length]
			args := ""
			if len(msg.Text) > et.Length {
				args = msg.Text[et.Offset + et.Length + 1:]
			}
			var realcmd string
			if msg.Chat.Type == "private" {
				realcmd = cmd
			} else {
				items := strings.Split(cmd, "@")
				if len(items) != 2 {
					lib.XLogErr("invalid command", cmd)
					continue
				}
				realcmd = items[0]
				target := items[1]
				config := model.GetMeConfig{}
				err := tb.Call(config)
				if err != nil {
					lib.XLogErr("GetMe", err)
					continue
				}
				if target != config.Response.UserName{
					lib.XLogErr("nothing to do", config, target)
					continue
				}
			}
			if len(args) == 0{
				lib.XLogInfo("empty args", et)
				continue
			}
			if realcmd == "addtopfeeds" {
				items := strings.Split(args, " ")
				if err := AddTopFeeds(items[0], items[1]); err != nil {
					lib.XLogErr("AddTopFeeds", err)
					continue
				}
				return
			} else if realcmd == "deltopfeeds"{
				if err := DelTopFeeds(args); err != nil {
					lib.XLogErr("DelTopFeeds", err)
					continue
				}
				return
			}

			str_id := args
			config := model.GetChatConfig{}
			if _, err := strconv.ParseInt(str_id, 10, 64); err == nil{
				config.ChatID = str_id
			} else {
				config.ChatID = "@" + str_id
			}
			err := tb.Call(&config)
			if err != nil {
				lib.XLogErr(err, config)
				continue
			}
			if realcmd == "addgroup"{
				err = Index(config.Response.UserName, config.Response.Title, config.Response.Description)
			} else if realcmd == "delgroup"{
				err = DeleteIndex(config.Response.UserName)
			}
			if err != nil {
				lib.XLogErr(err, realcmd, config.Response)
			}
		}
	}
}

func main(){
	InitConfig()

	tb := model.TBot{}
	tb.BotKey = g_sBotKey

	config := model.UpdateConfig{}
	config.Offset = 0
	config.Limit = 100
	config.Timeout = 10
	ch := tb.GetUpdateChan(&config)
	for update := range ch {
		if update.CallbackQuery != nil {
			go ProcessCallback(tb, update.CallbackQuery)
			continue
		}
		if update.Message != nil && len(update.Message.Text) > 0{
			if HasBotCommand(update.Message){
				go ProcessCommand(tb, update.Message)
			} else if update.Message.Chat.Type != "private" {
				go ProcessSearch(tb, update.Message)
			}
		}
	}
}
