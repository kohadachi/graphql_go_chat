package infrastructure

import (
	// "fmt"
	"log"
	"net/http"

	"github.com/99designs/gqlgen/handler"
	"github.com/go-redis/redis"
	"github.com/gorilla/websocket"
	"github.com/kouheiadachi/graphql_go_chat/graph"
	"github.com/rs/cors"
)

// GraphQLServer ..
type GraphQLServer struct {
	redisClient *redis.Client
}

// NewGraphQLServer returns GraphQL server.
func NewGraphQLServer(redisClient *redis.Client) *GraphQLServer {

	return &GraphQLServer{
		redisClient: redisClient,
	}
}

// Serve starts GraphQL server.
func (s *GraphQLServer) Serve(route string, port int) error {

	log.Println("runnning server...")

	mux := http.NewServeMux()
	mux.Handle(
		route,
		handler.GraphQL(graph.NewExecutableSchema(graph.NewGraphQLConfig(s.redisClient)),
			handler.WebsocketUpgrader(websocket.Upgrader{
				CheckOrigin: func(r *http.Request) bool {
					return true
				},
			}),
		),
	)

	mux.Handle("/", handler.Playground("GraphQL playground", route))

	handler := cors.AllowAll().Handler(mux)
	return http.ListenAndServe(":8081", handler)
}