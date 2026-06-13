package domain

import "fmt"

type EnvironmentConfig struct {
	Engine   string
	Host     string
	Port     int
	User     string
	Password string
}

func (c EnvironmentConfig) DSN() string {
	port := c.Port
	if port <= 0 {
		if c.Engine == "postgres" {
			port = 5432
		} else {
			port = 3306
		}
	}
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	if c.Engine == "postgres" {
		return fmt.Sprintf("postgres://%s:%s@%s:%d/?sslmode=disable", c.User, c.Password, host, port)
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/?parseTime=true", c.User, c.Password, host, port)
}

func (c EnvironmentConfig) PostgresDSN(database string) string {
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
