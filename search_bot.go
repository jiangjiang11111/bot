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
)

var g_sBotKey = "" 
var g_lSuperUser []string

func Index(groupid int64, title, des string)error{
	str_id := strconv.FormatInt(groupid, 10)
	key_title := title + "_" + str_id
	key_des := des + "_" + str_id
	err := db.Set(key_title, "1")
	if err != nil {
		lib.XLogErr(err, groupid, title)
		return err
	}
	err = db.Set(key_des, "1")
	if err != nil {
		lib.XLogErr(err, groupid, des)
		return err
	}
	err = db.Set(str_id + "_title", key_title)
	if err != nil {
		lib.XLogErr(err, str_id, key_title)
		return err
	}
	err = db.Set(str_id + "des", key_des)
	if err != nil {
		lib.XLogErr(err, str_id, key_des)
		return err
	}
	lib.XLogInfo("index", groupid, title, des)
	return nil
}

func DeleteIndex(groupid int64)error{
	str_id := strconv.FormatInt(groupid, 10)
	key_title, err := db.Get(str_id + "_title")
	if err != nil {
		lib.XLogErr("get key_title", err, str_id)
		return err
	}
	key_des, err := db.Get(str_id + "_des")
	if err != nil {
		lib.XLogErr("get key_des", err, str_id)
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

func UpdateIndex(groupid int64, title, des string)error{
	err := DeleteIndex(groupid)
	if err != nil {
		lib.XLogErr("DeleteIndex", groupid)
		return err
	}
	err = Index(groupid, title, des)
	if err != nil {
		lib.XLogErr("index", err, groupid, title, des)
		return err
	}
	return nil
}

func SearchIndex(keyword string, offset uint64, limit int64)([]int64, uint64, error){
	var ids []int64
	cmd := "*" + keyword + "*"
	keys, cursor, err := db.Search(cmd, offset, limit)
	if err != nil {
		lib.XLogErr("Search", err, keyword, offset, limit)
		return ids, cursor, err
	}
	lib.XLogInfo(cmd, offset, limit, keys)
	for _, key := range keys{
		vals := strings.Split(key, "_")
		if len(vals) == 0 {
			lib.XLogErr("invalid id", key)
			continue
		}
		str_id := vals[len(vals)-1]
		groupid, err := strconv.ParseInt(str_id, 10, 64)
		if err != nil {
			lib.XLogErr("ParseInt", err, str_id)
			continue
		}
		ids = append(ids, groupid)
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
		//fmt.Println(line)
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
		}
	}
}

func GetUTF16Len(content string)int{
	encodeContent := utf16.Encode([]rune(content))
	return len(encodeContent)
}

func ProcessSearch(tb model.TBot, msg *model.Message){
	keyword := msg.Text
	lib.XLogInfo(keyword)
	var cursor uint64
	var groupids []int64
	for{
		var err error
		var tmp_ids []int64
		tmp_ids, cursor, err = SearchIndex(keyword, cursor, 10)
		if err != nil {
			lib.XLogErr("SearchIndex", err, keyword, cursor)
			break
		}
		groupids = append(groupids, tmp_ids...)
		if cursor == 0 {
			break
		}
	}
	lib.XLogInfo(groupids)
	send_config := model.SendMessageConfig{}
	send_config.ChatID = msg.Chat.ID
	send_config.ReplyParams = model.ReplyParameters{MessageID:msg.MessageID}
	msg_content := "[\u5e7f\u544a] "
	url := model.MessageEntity{}
	url.Type = "text_link"
	url.URL = "https://t.me/callmemaybe1x"
	url.Offset = GetUTF16Len(msg_content)
	show_text := "\u56db\u4e24\u62e8\u5343\u65a4\u002c\u5c3d\u5728\u8461\u4eac\u83e0\u83dc\u7f51"
	url.Length = GetUTF16Len(show_text)
	send_config.Entities = append(send_config.Entities, url)
	msg_content += show_text + "\n"

	for i, id := range groupids {
		config := model.GetChatConfig{}
		config.ChatID = id

		lib.XLogInfo(config)
		err := tb.Call(&config)
		if err != nil {
			lib.XLogErr(err, config)
			continue
		}
		rsp := config.Response
		url := model.MessageEntity{}
		url.Type = "text_link"
		url.URL = "https://t.me/" + rsp.UserName
		url.Offset = GetUTF16Len(msg_content)
		show_text := strconv.Itoa(i + 1) + ". " + rsp.Title
		url.Length = GetUTF16Len(show_text)
		send_config.Entities = append(send_config.Entities, url)
		msg_content += show_text + "\n"
	}
	send_config.Text = msg_content
	send_config.LinkPreviewOption.IsDisable = true
	err := tb.Call(&send_config)
	if err != nil {
		lib.XLogErr(err, send_config)
	}
}

func ProcessCommand(tb model.TBot, msg *model.Message){
	for _, et := range msg.Entities {
		if et.Type == "bot_command" {
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
			// addgroup only
			if realcmd != "addgroup" {
				lib.XLogInfo("addgroup command only", realcmd)
				continue
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
			err = Index(config.Response.ID, config.Response.Title, config.Response.Description)
			if err != nil {
				lib.XLogErr("Index", err, config.Response)
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
		if update.Message != nil && len(update.Message.Text) > 0{
			if len(update.Message.Entities) > 0{
				go ProcessCommand(tb, update.Message)
			} else {
				go ProcessSearch(tb, update.Message)
			}
		}
	}
}
