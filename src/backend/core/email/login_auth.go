// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package email

import (
	"errors"
	"net/smtp"
)

// loginAuth implements smtp.Auth for the LOGIN mechanism.
type loginAuth struct {
	username, password string
}

// LoginAuth returns an Auth that implements the LOGIN authentication mechanism.
func LoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}

	command := string(fromServer)
	switch command {
	case "Username:", "Username", "username:":
		return []byte(a.username), nil
	case "Password:", "Password", "password:":
		return []byte(a.password), nil
	default:
		return nil, errors.New("unexpected LOGIN challenge")
	}
}
