package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/go-redis/redis"
	"github.com/kouheiadachi/graphql_go_chat/graph/generated"
	"github.com/kouheiadachi/graphql_go_chat/graph/model"
)

func (r *mutationResolver) PostMessage(ctx context.Context, user string, message string) (*model.Message, error) {
	isLogined, _ := r.checkLogin(user)
	if !isLogined {
		return nil, errors.New("This user does not exists")
	}
	// ユーザー情報はAFK(Away From Keyboard)対策で60minで削除されるようにしている。
	// メッセージの投稿を行った場合はExpireまでの時間をリセットする。
	val, _ := r.redisClient.SetXX(user, user, 60*time.Minute).Result()
	if val == false {
		return nil, errors.New("This user does not exists")
	}

	// 以下の部分で、[]byteに変換したMessageをredisのPubSubで配信しています。
	m := model.Message{
		User:    user,
		Message: message,
	}
	mb, _ := json.Marshal(m)
	r.redisClient.Publish("room", mb)
	return &m, nil
}

func (r *mutationResolver) CreateUser(ctx context.Context, user string) (string, error) {
	// This means that users has to call CreateUser again in 60 minutes.
	val, err := r.redisClient.SetNX(user, user, 60*time.Minute).Result()
	if err != nil {
		log.Println(err)
		return "", err
	}
	if val == false {
		return "", errors.New("This User name has already used")
	}

	// Notify new user joined.
	// TODO : Publish a notify through redis pub sub.
	r.mutex.Lock()
	for _, ch := range r.userChannels {
		ch <- user
	}
	r.mutex.Unlock()

	return user, nil
}

func (r *queryResolver) Users(ctx context.Context) ([]string, error) {
	users, err := r.redisClient.Keys("*").Result()
	if err != nil {
		log.Println(err)
		return nil, err
	}

	log.Println("【Query】Users : ", users)

	return users, nil
}

func (r *subscriptionResolver) MessagePosted(ctx context.Context, user string) (<-chan *model.Message, error) {
	isLogined, err := r.checkLogin(user)
	if err != nil {
		return nil, err
	}
	if !isLogined {
		return nil, errors.New("This user has not been created")
	}

	messageChan := make(chan *model.Message, 1)
	r.mutex.Lock()
	r.messageChannels[user] = messageChan
	r.mutex.Unlock()

	go func() {
		<-ctx.Done()
		r.mutex.Lock()
		delete(r.messageChannels, user)
		r.mutex.Unlock()
		r.redisClient.Del(user)
	}()

	log.Println("【Subscription】MessagePosted : ", user)

	return messageChan, nil
}

func (r *subscriptionResolver) UserJoined(ctx context.Context, user string) (<-chan string, error) {
	isLogined, err := r.checkLogin(user)
	if err != nil {
		return nil, err
	}
	if !isLogined {
		return nil, errors.New("This user has not been created")
	}

	userChan := make(chan string, 1)
	r.mutex.Lock()
	r.userChannels[user] = userChan
	r.mutex.Unlock()

	go func() {
		<-ctx.Done()
		r.mutex.Lock()
		delete(r.userChannels, user)
		r.mutex.Unlock()
		r.redisClient.Del(user)
	}()

	log.Println("【Subscription】UserJoined : ", user)

	return userChan, nil
}

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

// Subscription returns generated.SubscriptionResolver implementation.
func (r *Resolver) Subscription() generated.SubscriptionResolver { return &subscriptionResolver{r} }

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
type subscriptionResolver struct{ *Resolver }

// !!! WARNING !!!
// The code below was going to be deleted when updating resolvers. It has been copied here so you have
// one last chance to move it out of harms way if you want. There are two reasons this happens:
//  - When renaming or deleting a resolver the old code will be put in here. You can safely delete
//    it when you're done.
//  - You have helper methods in this file. Move them out to keep these resolver files clean.
func NewGraphQLConfig(redisClient *redis.Client) generated.Config {
	resolver := newResolver(redisClient)
	resolver.startSubscribingRedis()

	return generated.Config{
		Resolvers: resolver,
	}
}

type Resolver struct {
	redisClient     *redis.Client
	messageChannels map[string]chan model.Message
	userChannels    map[string]chan string
	mutex           sync.Mutex
}

func newResolver(redisClient *redis.Client) *Resolver {
	return &Resolver{
		redisClient:     redisClient,
		messageChannels: map[string]chan model.Message{},
		userChannels:    map[string]chan string{},
		mutex:           sync.Mutex{},
	}
}
func (r *Resolver) checkLogin(user string) (bool, error) {
	val, err := r.redisClient.Exists(user).Result()
	if err != nil {
		log.Println(err)
		return false, err
	}

	if val == 1 {
		return true, nil
	}
	return false, nil
}
func (r *Resolver) startSubscribingRedis() {
	log.Println("Start Subscribing Redis...")

	go func() {
		pubsub := r.redisClient.Subscribe("room")
		defer pubsub.Close()

		for {
			msgi, err := pubsub.Receive()
			if err != nil {
				panic(err)
			}

			switch msg := msgi.(type) {
			case *redis.Message:

				// Convert recieved string to Message.
				m := model.Message{}
				if err := json.Unmarshal([]byte(msg.Payload), &m); err != nil {
					log.Println(err)
					continue
				}

				// Notify new message.
				r.mutex.Lock()
				for _, ch := range r.messageChannels {
					ch <- m
				}
				r.mutex.Unlock()

			default:
			}
		}
	}()
}
