// Package controlplane implements the unified API gateway used by Myaaw
// clients. It is served by `myaaw api`.
//
// @title           Myaaw Control Plane API
// @version         0.1.0
// @description     Unified API for agents, chat, logs, and files. Authenticate with POST /auth/login, then send the token in the Authorization header.
// @host            localhost:9000
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
// @description     Token returned by POST /auth/login. Paste it with or without the "Bearer " prefix.
package controlplane
