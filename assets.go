package myaaw

import "embed"

//go:embed all:.myaaw
var DefaultMyaawDir embed.FS

//go:embed .env.example
var DefaultEnvExample string

//go:embed docker-compose.yml
var DefaultDockerCompose string
