package services

import (
	"bufio"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

type LinuxUser struct {
	Home     string `json:"home"`
	Name     string `json:"name"`
	Shell    string `json:"shell"`
	UID      int    `json:"uid"`
	Username string `json:"username"`
}

func HomeUsers() ([]LinuxUser, error) {
	entries, err := os.ReadDir("/home")
	if err != nil {
		if os.IsNotExist(err) {
			return []LinuxUser{}, nil
		}
		return nil, err
	}

	passwdUsers := passwdUsersByName()
	users := make([]LinuxUser, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		home := filepath.Join("/home", name)
		linuxUser := LinuxUser{
			Home:     home,
			Name:     name,
			Shell:    passwdUsers[name].Shell,
			UID:      -1,
			Username: name,
		}

		if lookup, err := user.Lookup(name); err == nil {
			linuxUser.Username = lookup.Username
			if uid, err := strconv.Atoi(lookup.Uid); err == nil {
				linuxUser.UID = uid
			}
		} else if passwdUsers[name].Username != "" {
			linuxUser.Username = passwdUsers[name].Username
			linuxUser.UID = passwdUsers[name].UID
		}

		users = append(users, linuxUser)
	}

	return users, nil
}

type passwdUser struct {
	Shell    string
	UID      int
	Username string
}

func passwdUsersByName() map[string]passwdUser {
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return map[string]passwdUser{}
	}
	defer file.Close()

	users := make(map[string]passwdUser)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) < 7 {
			continue
		}

		uid, _ := strconv.Atoi(parts[2])
		users[parts[0]] = passwdUser{
			Shell:    parts[6],
			UID:      uid,
			Username: parts[0],
		}
	}

	return users
}
