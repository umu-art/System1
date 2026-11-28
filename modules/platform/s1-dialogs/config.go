package main

type Config struct {
	Port        string `env:"PORT" envDefault:"8080"`
	Env         string `env:"S1_ENV" envDefault:"local"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	DatabaseURL string `env:"DATABASE_URL,required"`
}
