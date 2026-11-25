package api

func (s *Server) publicRoutes(route router) {
	route("GET /api/health", s.health)
}
