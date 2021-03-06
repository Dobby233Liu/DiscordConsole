/*
DiscordConsole is a software aiming to give you full control over accounts, bots and webhooks!
Copyright (C) 2017  LEGOlord208

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/
package main

import (
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/bwmarrin/discordgo"
	"github.com/discordconsole-team/DiscordConsole/PermCalc"
	"github.com/fatih/color"
	"github.com/legolord208/gtable"
	"github.com/legolord208/stdutil"
)

var mutexCommand sync.Mutex

var lastUsedMsg string
var lastUsedRole string

var cacheRead *discordgo.Message
var cacheUser []*keyval
var cacheInvite []*keyval

var messages = messagesNone
var intercept = true
var output = false

var aliases map[string]string

var webhookCommands = []string{"help", "big", "say", "sayfile", "embed", "name", "avatar", "exit", "exec", "run", "lang"}

func command(session *discordgo.Session, source commandSource, cmd string, w io.Writer) (returnVal string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	parts := strings.FieldsFunc(cmd, func(c rune) bool {
		return c != '\n' && unicode.IsSpace(c)
	})

	cmd = strings.ToLower(parts[0])
	args := parts[1:]

	returnVal = commandRaw(session, source, cmd, args, w)
	return
}

func commandRaw(session *discordgo.Session, source commandSource, cmd string, args []string, w io.Writer) (returnVal string) {
	defer handleCrash()
	nargs := len(args)

	if !source.NoMutex {
		mutexCommand.Lock()
		defer mutexCommand.Unlock()
	}

	if aliascmd, ok := aliases[cmd]; ok && !source.Alias && cmd != "alias" {
		if nargs >= 1 {
			aliascmd += " " + strings.Join(args, " ")
		}
		colors := w == color.Output
		if colors {
			colorAutomated.Set()
		}
		writeln(w, aliascmd)
		if colors {
			color.Unset()
		}

		// Won't use source anywhere else.
		// No reason to copy the variable.
		source.Alias = true
		source.NoMutex = true
		return command(session, source, aliascmd, w)
	}

	if userType == typeWebhook {
		allowed := false
		for _, allow := range webhookCommands {
			if cmd == allow {
				allowed = true
			}
		}

		if !allowed {
			stdutil.PrintErr(tl("invalid.webhook.command"), nil)
			return
		}
	}

	switch cmd {
	case "help":
		search := strings.Join(args, " ")
		printHelp(search)
	case "exit":
		closing = true
	case "exec":
		if nargs < 1 {
			stdutil.PrintErr("exec <command>", nil)
			return
		}

		cmd := strings.Join(args, " ")

		err := execute(sh, c, cmd)
		if err != nil {
			stdutil.PrintErr(tl("failed.exec"), err)
		}
	case "run":
		if nargs < 1 {
			stdutil.PrintErr("run <lua script>", nil)
			return
		}
		var script string
		var scriptArgs []string

		scriptName := true
		for i, arg := range args {
			if scriptName {
				if i != 0 {
					script += " "
				}
				if strings.HasSuffix(arg, ":") {
					scriptName = false
					arg = arg[:len(arg)-1]
				}
				script += arg
			} else {
				scriptArgs = append(scriptArgs, arg)
			}
		}

		err := fixPath(&script)
		if err != nil {
			stdutil.PrintErr(tl("failed.fixpath"), err)
		}

		mutexCommand.Unlock()
		err = runLua(session, script, scriptArgs...)
		mutexCommand.Lock()
		if err != nil {
			stdutil.PrintErr(tl("failed.lua.run"), err)
		}
	case "lang":
		if nargs < 1 {
			stdutil.PrintErr("lang <language>", nil)
			return
		}

		loadLangAuto(args[0])
	case "guilds", "guild", "channels", "pchannels", "vchannels",
		"channel", "dm", "bookmarks", "bookmark", "go":
		returnVal = commandsNavigate(session, cmd, args, nargs, w)
	case "say", "tts", "embed", "quote", "big", "file", "edit",
		"editembed", "sayfile", "del", "delall":
		returnVal = commandsSay(session, source, cmd, args, nargs, w)
	case "log":
		if loc.channel == nil {
			stdutil.PrintErr(tl("invalid.channel"), nil)
			return
		}

		directly := nargs < 1

		var file io.Writer

		if directly {
			file = w
		} else {
			name := strings.Join(args, " ")
			err := fixPath(&name)
			if err != nil {
				stdutil.PrintErr(tl("failed.fixpath"), err)
			}

			file2, err := os.Create(name)
			if err != nil {
				stdutil.PrintErr(tl("failed.file.open"), err)
				return
			}
			defer file2.Close()

			file = file2
		}

		limit := 100
		if directly {
			limit = 10
		}

		msgs, err := session.ChannelMessages(loc.channel.ID, limit, "", "", "")
		if err != nil {
			stdutil.PrintErr(tl("failed.msg.query"), err)
			return
		}

		for i := len(msgs) - 1; i >= 0; i-- {
			msg := msgs[i]
			if msg.Author == nil {
				return
			}
			s := ""
			if directly {
				s = "(ID " + msg.ID + ") "
			}
			err = writeln(file, s+msg.Author.Username+": "+msg.Content)
			if err != nil && !directly {
				stdutil.PrintErr(tl("failed.msg.write"), err)
				return
			}
		}
	case "members":
		if loc.guild == nil {
			stdutil.PrintErr(tl("invalid.guild"), nil)
			return
		}

		members, err := session.GuildMembers(loc.guild.ID, "", 100)
		if err != nil {
			stdutil.PrintErr(tl("failed.members"), err)
			return
		}

		table := gtable.NewStringTable()
		table.AddStrings("ID", "Name", "Nick")

		for _, member := range members {
			table.AddRow()
			table.AddStrings(member.User.ID, member.User.String(), member.Nick)
		}
		writeln(w, table.String())
	case "invite":
		if nargs < 1 {
			stdutil.PrintErr("invite accept <code> OR invite create [expire] [max uses] ['temp']", nil)
			return
		}
		switch strings.ToLower(args[0]) {
		case "see", "read":
			if nargs < 2 {
				stdutil.PrintErr("invite see <code> [property]", nil)
				return
			}

			var keyvals []*keyval
			if strings.EqualFold(args[1], "cache") {
				if cacheInvite == nil {
					stdutil.PrintErr(tl("invalid.cache"), nil)
					return
				}

				keyvals = cacheInvite
			} else {
				invite, err := session.Invite(args[1])
				if err != nil {
					stdutil.PrintErr(tl("failed.invite"), err)
					return
				}

				keyvals = invite2array(invite)
				cacheInvite = keyvals
			}

			if nargs < 3 {
				for _, keyval := range keyvals {
					writeln(w, keyval.String())
				}
			} else {
				var ok bool
				returnVal, ok = findValByKey(keyvals, args[2])
				if !ok {
					stdutil.PrintErr(tl("invalid.value"), nil)
					return
				}

				writeln(w, returnVal)
			}
		case "accept":
			if nargs < 2 {
				stdutil.PrintErr("invite accept <code>", nil)
				return
			}
			if userType != typeUser {
				stdutil.PrintErr(tl("invalid.onlyfor.users"), nil)
				return
			}

			invite, err := session.InviteAccept(args[1])
			if err != nil {
				stdutil.PrintErr(tl("failed.invite.accept"), err)
				return
			}
			writeln(w, tl("status.invite.accept"))

			loc.push(invite.Guild, invite.Channel)
		case "create":
			if loc.channel == nil {
				stdutil.PrintErr(tl("failed.channel"), nil)
				return
			}

			inviteObj := discordgo.Invite{}
			if nargs >= 2 {
				min, err := strconv.Atoi(args[1])
				if err != nil {
					stdutil.PrintErr(tl("invalid.number"), nil)
					return
				}
				inviteObj.MaxAge = 60 * min
				if nargs >= 3 {
					num, err := strconv.Atoi(args[2])
					if err != nil {
						stdutil.PrintErr(tl("invalid.number"), nil)
						return
					}
					inviteObj.MaxUses = num

					if nargs >= 4 && strings.EqualFold(args[3], "temp") {
						inviteObj.Temporary = true
					}
				}
			}

			invite, err := session.ChannelInviteCreate(loc.channel.ID, inviteObj)
			if err != nil {
				stdutil.PrintErr(tl("failed.invite.create"), err)
				return
			}
			writeln(w, tl("status.invite.create")+" "+invite.Code)
			returnVal = invite.Code
		default:
			stdutil.PrintErr(tl("invalid.value"), nil)
		}
	case "messages":
		if len(args) < 1 {
			messages = messagesCurrent
			return
		}

		val, ok := typeMessages[strings.ToLower(args[0])]
		if !ok {
			stdutil.PrintErr(tl("invalid.value"), nil)
			return
		}
		messages = val
	case "intercept":
		if len(args) < 1 {
			intercept = !intercept
			returnVal = strconv.FormatBool(intercept)
			writeln(w, returnVal)
			return
		}

		state, err := parseBool(args[0])
		if err != nil {
			stdutil.PrintErr("", err)
			return
		}
		intercept = state
	case "output":
		if len(args) < 1 {
			output = !output
			returnVal = strconv.FormatBool(output)
			writeln(w, returnVal)
			return
		}

		state, err := parseBool(args[0])
		if err != nil {
			stdutil.PrintErr("", err)
			return
		}
		output = state
	case "back":
		loc.push(lastLoc.guild, lastLoc.channel)
	case "ban":
		if nargs < 1 {
			stdutil.PrintErr("ban <user id>", nil)
			return
		}
		if loc.guild == nil {
			stdutil.PrintErr(tl("invalid.guild"), nil)
			return
		}

		err := session.GuildBanCreate(loc.guild.ID, args[0], 0)
		if err != nil {
			stdutil.PrintErr(tl("failed.ban.create"), err)
		}
	case "unban":
		if nargs < 1 {
			stdutil.PrintErr("unban <user id>", nil)
			return
		}
		if loc.guild == nil {
			stdutil.PrintErr(tl("invalid.guild"), nil)
			return
		}

		err := session.GuildBanDelete(loc.guild.ID, args[0])
		if err != nil {
			stdutil.PrintErr(tl("failed.ban.delete"), err)
		}
	case "kick":
		if nargs < 1 {
			stdutil.PrintErr("kick <user id>", nil)
			return
		}
		if loc.guild == nil {
			stdutil.PrintErr(tl("invalid.guild"), nil)
			return
		}

		err := session.GuildMemberDelete(loc.guild.ID, args[0])
		if err != nil {
			stdutil.PrintErr(tl("failed.kick"), err)
		}
	case "leave":
		if loc.guild == nil {
			stdutil.PrintErr(tl("invalid.guild"), nil)
			return
		}

		err := session.GuildLeave(loc.guild.ID)
		if err != nil {
			stdutil.PrintErr(tl("failed.leave"), err)
			return
		}

		loc.push(nil, nil)
	case "bans":
		if loc.guild == nil {
			stdutil.PrintErr(tl("invalid.guild"), nil)
			return
		}

		bans, err := session.GuildBans(loc.guild.ID)
		if err != nil {
			stdutil.PrintErr(tl("failed.ban.list"), err)
			return
		}

		table := gtable.NewStringTable()
		table.AddStrings("User ID", "Username", "Reason")

		for _, ban := range bans {
			table.AddRow()
			table.AddStrings(ban.User.ID, ban.User.Username, ban.Reason)
		}

		writeln(w, table.String())
	case "nickall":
		if loc.guild == nil {
			stdutil.PrintErr(tl("invalid.guild"), nil)
			return
		}

		members, err := session.GuildMembers(loc.guild.ID, "", 100)
		if err != nil {
			stdutil.PrintErr(tl("failed.members"), err)
			return
		}

		nick := strings.Join(args, " ")

		for _, member := range members {
			err := session.GuildMemberNickname(loc.guild.ID, member.User.ID, nick)
			if err != nil {
				stdutil.PrintErr(tl("failed.nick"), err)
			}
		}
	case "play":
		if userType != typeBot {
			stdutil.PrintErr(tl("invalid.onlyfor.bots"), nil)
			return
		}
		if nargs < 1 {
			stdutil.PrintErr("play <dca audio file>", nil)
			return
		}
		if vc == nil {
			stdutil.PrintErr(tl("invalid.channel.voice"), nil)
			return
		}
		if playing != "" {
			stdutil.PrintErr(tl("invalid.music.playing"), nil)
			return
		}

		file := strings.Join(args, " ")
		err := fixPath(&file)
		if err != nil {
			stdutil.PrintErr(tl("failed.fixpath"), err)
		}

		playing = file

		writeln(w, tl("status.loading"))

		var buffer [][]byte
		err = loadAudio(file, &buffer)
		if err != nil {
			stdutil.PrintErr(tl("failed.file.load"), err)
			playing = ""
			return
		}

		writeln(w, "Loaded!")
		writeln(w, "Playing!")

		go func(buffer [][]byte, session *discordgo.Session, guild, channel string) {
			play(buffer, session, guild, channel)
			playing = ""
		}(buffer, session, loc.guild.ID, loc.channel.ID)
	case "stop":
		if userType != typeBot {
			stdutil.PrintErr(tl("invalid.onlyfor.bots"), nil)
			return
		}
		playing = ""
	case "reactadd":
		fallthrough
	case "reactdel":
		if nargs < 2 {
			stdutil.PrintErr("reactadd/reactdel <message id> <emoji unicode/id>", nil)
			return
		}
		if loc.channel == nil {
			stdutil.PrintErr(tl("invalid.channel"), nil)
			return
		}

		var err error
		if cmd == "reactadd" {
			err = session.MessageReactionAdd(loc.channel.ID, args[0], args[1])
		} else {
			err = session.MessageReactionRemove(loc.channel.ID, args[0], args[1], "@me")
		}
		if err != nil {
			stdutil.PrintErr(tl("failed.react"), err)
			return
		}
	case "block":
		if nargs < 1 {
			stdutil.PrintErr("block <user id>", nil)
			return
		}
		if userType != typeUser {
			stdutil.PrintErr(tl("invalid.onlyfor.users"), nil)
			return
		}
		err := session.RelationshipUserBlock(args[0])
		if err != nil {
			stdutil.PrintErr(tl("failed.block"), err)
			return
		}
	case "friends":
		if userType != typeUser {
			stdutil.PrintErr(tl("invalid.onlyfor.users"), nil)
			return
		}
		relations, err := session.RelationshipsGet()
		if err != nil {
			stdutil.PrintErr(tl("failed.friends"), err)
			return
		}

		table := gtable.NewStringTable()
		table.AddStrings("ID", "Type", "Name")

		for _, relation := range relations {
			table.AddRow()
			table.AddStrings(relation.ID, typeRelationships[relation.Type], relation.User.Username)
		}

		writeln(w, table.String())
	case "reactbig":
		if nargs < 2 {
			stdutil.PrintErr("reactbig <message id> <text>", nil)
			return
		}
		if loc.channel == nil {
			stdutil.PrintErr(tl("invalid.channel"), nil)
			return
		}

		used := ""

		for _, c := range strings.Join(args[1:], " ") {
			str := string(toEmoji(c))

			if strings.Contains(used, str) {
				writeln(w, tl("failed.react.used"))
				continue
			}
			used += str

			err := session.MessageReactionAdd(loc.channel.ID, args[0], str)
			if err != nil {
				stdutil.PrintErr(tl("failed.react"), err)
			}
		}
	case "rl":
		full := nargs >= 1 && strings.EqualFold(args[0], "full")

		var err error
		if full {
			writeln(w, tl("rl.session"))
			session.Close()
			err = session.Open()
			if err != nil {
				stdutil.PrintErr(tl("failed.session.start"), err)
			}
		}

		writeln(w, tl("rl.cache.loc"))
		var guild *discordgo.Guild
		var channel *discordgo.Channel

		if loc.guild != nil {
			guild, err = session.Guild(loc.guild.ID)

			if err != nil {
				stdutil.PrintErr(tl("failed.guild"), err)
				return
			}
		}

		if loc.channel != nil {
			channel, err = session.Channel(loc.channel.ID)

			if err != nil {
				stdutil.PrintErr(tl("failed.channel"), err)
				return
			}
		}

		loc.guild = guild
		loc.channel = channel
		pointerCache = ""

		writeln(w, tl("rl.cache.vars"))
		cacheGuilds = nil
		cacheChannels = nil
		cacheAudio = make(map[string][][]byte)
		bookmarksCache = make(map[string]*location)

		lastLoc = &location{}
		lastUsedMsg = ""
		lastUsedRole = ""

		cacheRead = nil
		cacheUser = nil
	case "status":
		if nargs < 1 {
			stdutil.PrintErr("status <value>", nil)
			return
		}
		if userType != typeUser {
			stdutil.PrintErr(tl("invalid.onlyfor.users"), nil)
			return
		}

		status, ok := typeStatuses[strings.ToLower(args[0])]
		if !ok {
			stdutil.PrintErr(tl("invalid.value"), nil)
			return
		}

		if status == discordgo.StatusOffline {
			stdutil.PrintErr(tl("invalid.status.offline"), nil)
			return
		}

		_, err := session.UserUpdateStatus(status)
		if err != nil {
			stdutil.PrintErr(tl("failed.status"), err)
			return
		}
		writeln(w, tl("status.status"))
	case "avatar", "name", "playing", "streaming", "typing", "nick":
		returnVal = commandsUserMod(session, cmd, args, nargs, w)
	case "read", "cinfo", "ginfo", "uinfo":
		returnVal = commandsQuery(session, cmd, args, nargs, w)
	case "roles", "roleadd", "roledel", "rolecreate", "roleedit", "roledelete":
		returnVal = commandsRoles(session, cmd, args, nargs, w)
	case "api_start":
		if apiName != "" {
			stdutil.PrintErr(tl("invalid.api.started"), nil)
			return
		}

		var name string
		if nargs >= 1 {
			name = strings.Join(args, " ")
			go apiStartName(session, name)
		} else {
			var err error
			name, err = apiStart(session)
			if err != nil {
				stdutil.PrintErr(tl("failed.api.start"), err)
				return
			}
		}
		writeln(w, tl("status.api.start")+" "+name)
		returnVal = name
	case "broadcast":
		if nargs < 1 {
			stdutil.PrintErr("broadcast <command>", nil)
			return
		}
		if apiName == "" {
			stdutil.PrintErr(tl("invalid.api.notstarted"), nil)
			return
		}

		err := apiSend(strings.Join(args, " "))
		if err != nil {
			stdutil.PrintErr(tl("failed.generic"), err)
			return
		}
		source.NoMutex = true
		return commandRaw(session, source, args[0], args[1:], w)
	case "api_stop":
		apiStop()
	case "region":
		if nargs < 1 {
			stdutil.PrintErr("region list OR region set <region>", nil)
			return
		}
		switch strings.ToLower(args[0]) {
		case "list":
			regions, err := session.VoiceRegions()
			if err != nil {
				stdutil.PrintErr(tl("failed.voice.regions"), err)
				return
			}

			table := gtable.NewStringTable()
			table.AddStrings("ID", "Name", "Port")

			for _, region := range regions {
				table.AddRow()
				table.AddStrings(region.ID, region.Name, strconv.Itoa(region.Port))
			}

			writeln(w, table.String())
		case "set":
			if nargs < 2 {
				stdutil.PrintErr("region set <region>", nil)
				return
			}
			if loc.guild == nil {
				stdutil.PrintErr(tl("invalid.guild"), nil)
				return
			}

			_, err := session.GuildEdit(loc.guild.ID, discordgo.GuildParams{
				Region: args[1],
			})
			if err != nil {
				stdutil.PrintErr(tl("failed.guild.edit"), err)
			}
		}
	case "alias":
		if nargs <= 0 {
			for alias, aliascmd := range aliases {
				writeln(w, alias+"=`"+aliascmd+"`")
			}
		} else if nargs == 1 {
			delete(aliases, strings.ToLower(args[0]))
		} else {
			if aliases == nil {
				aliases = make(map[string]string)
			}

			aliases[strings.ToLower(args[0])] = strings.Join(args[1:], " ")
		}
	case "permcalc":
		if !source.Terminal {
			stdutil.PrintErr(tl("invalid.source.terminal"), nil)
			return
		}
		pm := permcalc.PermCalc{}

		if nargs >= 1 {
			i, err := strconv.Atoi(args[0])
			if err != nil {
				stdutil.PrintErr(tl("invalid.number"), nil)
				return
			}

			pm.Perm = i
		}

		err := pm.Show()
		if err != nil {
			stdutil.PrintErr("failed.permcalc", err)
			return
		}
		writeln(w, strconv.Itoa(pm.Perm))
	case "crash":
		if nargs >= 1 && strings.EqualFold(args[0], "die") {
			// Make error async for more "damage"
			go func() {
				panic("die")
			}()
			return
		}
		panic("triggered crash")
	default:
		stdutil.PrintErr(tl("invalid.command"), nil)
	}
	return
}

func parseBool(str string) (bool, error) {
	if str == "yes" || str == "true" || str == "y" {
		return true, nil
	} else if str == "no" || str == "false" || str == "n" {
		return false, nil
	}
	return false, errors.New(tl("invalid.yn"))
}
