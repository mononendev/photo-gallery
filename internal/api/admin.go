package api

func (s *Server) adminRoutes(route router) {
	route("GET /api/health", s.health)
}
