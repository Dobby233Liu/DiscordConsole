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
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/legolord208/stdutil"
)

func commandsUserMod(session *discordgo.Session, cmd string, args []string, nargs int, w io.Writer) (returnVal string) {
	switch cmd {
	case "avatar":
		if nargs < 1 {
			stdutil.PrintErr("avatar <file/link>", nil)
			return
		}

		var reader io.Reader
		resource := strings.Join(args, " ")

		if strings.HasPrefix(resource, "https://") || strings.HasPrefix(resource, "http://") {
			res, err := http.Get(resource)
			if err != nil {
				stdutil.PrintErr(tl("failed.webrequest"), err)
				return
			}
			defer res.Body.Close()

			reader = res.Body
		} else {
			err := fixPath(&resource)
			if err != nil {
				stdutil.PrintErr(tl("failed.fixpath"), err)
				return
			}

			r, err := os.Open(resource)
			defer r.Close()
			if err != nil {
				stdutil.PrintErr(tl("failed.file.open"), err)
				return
			}

			reader = r
		}

		writer := bytes.NewBuffer([]byte{})
		b64 := base64.NewEncoder(base64.StdEncoding, writer)

		_, err := io.Copy(b64, reader)
		if err != nil {
			stdutil.PrintErr(tl("failed.base64"), err)
			return
		}
		b64.Close()

		// Too lazy to detect image type. Seems to work anyway ¯\_(ツ)_/¯
		str := "data:image/png;base64," + writer.String()

		if userType == typeWebhook {
			_, err = session.WebhookEditWithToken(userID, userToken, "", str)
			if err != nil {
				stdutil.PrintErr(tl("failed.avatar"), err)
				return
			}
			return
		}

		user, err := session.User("@me")
		if err != nil {
			stdutil.PrintErr(tl("failed.user"), err)
			return
		}

		_, err = session.UserUpdate("", "", user.Username, str, "")
		if err != nil {
			stdutil.PrintErr(tl("failed.avatar"), err)
			return
		}
		writeln(w, tl("status.avatar"))
	case "name":
		if nargs < 1 {
			stdutil.PrintErr("name <handle>", nil)
			return
		}

		if userType == typeWebhook {
			_, err := session.WebhookEditWithToken(userID, userToken, strings.Join(args, " "), "")
			if err != nil {
				stdutil.PrintErr(tl("failed.user.edit"), err)
			}
			return
		}

		user, err := session.User("@me")
		if err != nil {
			stdutil.PrintErr(tl("failed.user"), err)
			return
		}

		user, err = session.UserUpdate("", "", strings.Join(args, " "), user.Avatar, "")
		if err != nil {
			stdutil.PrintErr(tl("failed.user.edit"), err)
			return
		}
		writeln(w, tl("status.name"))
	case "playing":
		err := session.UpdateStatus(0, strings.Join(args, " "))
		if err != nil {
			stdutil.PrintErr(tl("failed.status"), err)
		}
	case "streaming":
		var err error
		if nargs <= 0 {
			err = session.UpdateStreamingStatus(0, "", "")
		} else if nargs < 2 {
			err = session.UpdateStreamingStatus(0, strings.Join(args[1:], " "), "")
		} else {
			err = session.UpdateStreamingStatus(0, strings.Join(args[1:], " "), args[0])
		}
		if err != nil {
			stdutil.PrintErr(tl("failed.status"), err)
		}
	case "typing":
		if loc.channel == nil {
			stdutil.PrintErr(tl("failed.channel"), nil)
			return
		}
		err := session.ChannelTyping(loc.channel.ID)
		if err != nil {
			stdutil.PrintErr(tl("failed.typing"), err)
		}
	case "nick":
		if loc.guild == nil {
			stdutil.PrintErr(tl("invalid.guild"), nil)
			return
		}
		if nargs < 1 {
			stdutil.PrintErr("nick <id> [nickname]", nil)
			return
		}

		who := args[0]
		if strings.EqualFold(who, "@me") {
			who = "@me/nick"
			// Should hopefully only be @me in the future.
			// See https://github.com/bwmarrin/discordgo/issues/318
		}

		err := session.GuildMemberNickname(loc.guild.ID, who, strings.Join(args[1:], " "))
		if err != nil {
			stdutil.PrintErr(tl("failed.nick"), err)
		}
	}
	return
}
