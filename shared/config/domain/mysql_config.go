package domain

import "fmt"

type MySQLConfig struct {
	Host     string
	Port     int
	User     string
	Password string
}

func (c MySQLConfig) DSN() string {
	port := c.Port
	if port <= 0 {
		port = 3306
	}
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/?parseTime=true", c.User, c.Password, host, port)
}
