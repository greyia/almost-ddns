package main

import (
	"flag"
	"fmt"
	"github.com/bluele/slack"
	"github.com/goji/httpauth"
	"github.com/zenazn/goji"
	"net"

	"github.com/BurntSushi/toml"
	"github.com/ccding/go-stun/stun"
	_ "github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"os/exec"
	"path"
	"time"
)

type tomlConfig struct {
	Slack  slackConfig
	Server serverConfig
	Web    webConfig
}

type slackConfig struct {
	Channel string
	Token   string
}

type serverConfig struct {
	TargetDomain string
	NameServer   string
}

type webConfig struct {
	User string
	Pass string
	Bind string
}

type serverStatus struct {
	SameCount  int64
	CheckCount int64
}

var status serverStatus
var config tomlConfig

func main() {

	// move binary directory
	dir := path.Dir(os.Args[0])
	os.Chdir(dir)

	// load config
	if _, err := toml.DecodeFile("config.toml", &config); err != nil {
		panic(err)
	}

	flag.Set("bind", config.Web.Bind)
	go work()
	goji.Use(httpauth.SimpleBasicAuth(config.Web.User, config.Web.Pass))
	goji.Get("/v1/stat", StatAPIContoller)
	goji.Serve()

}

func work() {

	targetDomain := config.Server.TargetDomain
	nameServer := config.Server.NameServer

	// counter
	status.SameCount = 0
	status.CheckCount = 0

	// connection slack-api
	api := slack.New(config.Slack.Token)
	channel, err := api.FindChannelByName(config.Slack.Channel)
	if err != nil {
		panic(err)
	}

	for {
		status.CheckCount = status.CheckCount + 1

		// notify running?
		if status.CheckCount%60 == 0 {
			err = api.ChatPostMessage(channel.Id, "@takoyaki I'm running. ", nil)
			if err != nil {
				fmt.Println("slack api error", err)
			}
		}

		time.Sleep(60 * time.Second)

		// nonreq-resovle using dig(/hack/resolve.sh), get registry ipv4 address
		ripaddr, err := resolve(targetDomain, nameServer)
		if err != nil {
			fmt.Println("resolve error:", err)
			continue
		}

		if ripaddr == nil {
			fmt.Println("resolve error:", "resolve ip == nil")
			continue
		}

		fmt.Println("resovle ipaddr:", ripaddr.String())

		// get global-ipv4-address using STUN
		_, host, err := stun.NewClient().Discover()
		if err != nil {
			fmt.Println("stun client error", err)
			continue
		}

		if host == nil {
			fmt.Println("stun client error", "host == nil")
			continue
		}

		gipaddr := net.ParseIP(host.IP())
		fmt.Println("stun client ipaddr:", gipaddr.String())

		// compare
		if gipaddr.String() == ripaddr.String() {
			fmt.Println("same")
			status.SameCount = status.SameCount + 1
			continue
		}

		// notify update
		err = api.ChatPostMessage(channel.Id, "@takoyaki global-ipv4-address != resolve-ipv4-address. Now, I try to update. ", nil)
		if err != nil {
			fmt.Println("slack api error", err)
		}
		fmt.Println("not equal ipaddr")

		// update
		_, err = exec.Command("/bin/bash", "hack/update.sh", gipaddr.String()).Output()
		if err != nil {
			fmt.Println("update error", err)
			continue
		}

		// wait
		time.Sleep(5 * 10 * time.Second)

	}
}
