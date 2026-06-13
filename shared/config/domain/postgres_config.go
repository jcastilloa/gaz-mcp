package domain

import "fmt"

type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
}

func (c PostgresConfig) DSN(database string) string {
	port := c.Port
	if port <= 0 {
		port = 5432
	}
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", c.User, c.Password, host, port, database)
}
