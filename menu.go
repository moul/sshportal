package main

import (
	"fmt"
	"io"
	"strconv"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/gliderlabs/ssh"
	"github.com/jinzhu/gorm"
)

var (
	banner = `

    __________ _____           __       __
   / __/ __/ // / _ \___  ____/ /____ _/ /
  _\ \_\ \/ _  / ___/ _ \/ __/ __/ _ '/ /
 /___/___/_//_/_/   \___/_/  \__/\_,_/_/

`
	msgMenuRegisterHost = "Register Host"
	msgMenuExit         = "Exit"
	msgBye              = "Bye!\n"
)

func userMenu(s ssh.Session, db *gorm.DB) error {
	io.WriteString(s, banner)

	currentUser, err := getCurrentUser(s, db)
	if err != nil {
		return err
	}

	io.WriteString(s, fmt.Sprintf("Welcome %s!\n\n", currentUser.Name()))
	term := terminal.NewTerminal(s, "> ")

	switch inputRadio(term, "Main menu", []string{"Manage Hosts", "Manage Keys", "Manage Users"}) {
	case "Manage Hosts":
		switch inputRadio(term, "Manage Hosts", []string{"List", "Add", "Remove"}) {
		case "List":
			return fmt.Errorf("not implemented")
		case "Add":
			url := inputText(term, "Connection string (<user>@<hostname>:<port>)?", "root@127.0.0.1:22")
			fmt.Println(url)
			switch inputRadio(term, "Authentication method?", []string{"Password", "Key"}) {
			case "Password":

			case "Key":
				return fmt.Errorf("not implemented")
			}
		case "Remove":
			return fmt.Errorf("not implemented")
		}
	case "Manage Keys":
		return fmt.Errorf("not implemented")
	case "Manage Users":
		return fmt.Errorf("not implemented")
	}
	return nil
}

func inputText(term *terminal.Terminal, prompt, def string) string {
	term.Write([]byte(fmt.Sprintf("%s [%s]\n", prompt, def)))
	line, err := term.ReadLine()
	if err != nil {
		term.Write([]byte(fmt.Sprintf("error: %v\n", err)))
		return ""
	}
	if line == "" {
		return def
	}
	return line
}

func inputRadio(term *terminal.Terminal, prompt string, options []string) string {
	for {
		term.Write([]byte(fmt.Sprintf("%s\n", prompt)))
		for idx, option := range options {
			term.Write([]byte(fmt.Sprintf("  %d)  %s\n", idx+1, option)))
		}
		line, err := term.ReadLine()
		if err != nil {
			term.Write([]byte(fmt.Sprintf("error: %v\n", err)))
			return ""
		}
		choice, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			term.Write([]byte("error: not a number\n"))
			continue
		}
		if choice < 1 || int(choice) > len(options) {
			term.Write([]byte("error: invalid number\n"))
			continue
		}
		return options[choice-1]
	}
	return ""
}
