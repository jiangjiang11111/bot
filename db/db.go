package db

import (
    "context"
    "github.com/redis/go-redis/v9"
	"encoding/json"
	"errors"
	"log"
)

var ctx = context.Background()

var g_redis_cli = redis.NewClient(&redis.Options{
        Addr:     "localhost:6379",
        Password: "", // no password set
        DB:       0,  // use default DB
    })

func Set(key, value string) error{
	return g_redis_cli.Set(ctx, key, value, 0).Err();
}

func Del(key string) error {
	return g_redis_cli.Del(ctx, key).Err()
}

func Get(key string) (string, error){
	val, err := g_redis_cli.Get(ctx, key).Result()
	if err == redis.Nil {
		return val, errors.New("key not exists")
	} else if err != nil {
		return val, err
	} else {
		return val, nil
	}
}

func IsValueInSet(key, value string)(bool, error){
	return g_redis_cli.SIsMember(ctx, key, value).Result()
}

func AddToSet(key string, members ...interface{})(error){
	return g_redis_cli.SAdd(ctx, key, members).Err()
}

func GetSetMembers(key string)([]string, error){
	return g_redis_cli.SMembers(ctx, key).Result()
}

func DelFromSet(key string, members ...interface{})(error){
	return g_redis_cli.SRem(ctx, key, members).Err()
}

func GetSetCount(key string)(int64, error){
	return g_redis_cli.SCard(ctx, key).Result()
}

func SetStruct(key string, obj interface{})(error){
	str, err := json.Marshal(&obj)
    if err != nil {
		return err
	}
	return Set(key, string(str))
}

func GetStruct(key string, obj interface{})(error){
	str, err := Get(key)
	if err != nil{
		return err
	}
	err = json.Unmarshal([]byte(str), obj)
    if err != nil {
		return err
	}
	return nil
}

func Search(pattern string, offset uint64, limit int64)([]string, uint64, error){
	var keys []string
	var err error
	keys, cursor, err := g_redis_cli.Scan(ctx, offset, pattern, limit).Result()
	if err != nil {
		return keys, 0, err
	}
	log.Println(pattern, offset, limit, keys, cursor)
	return keys, cursor, nil
}

func BatchGetStruct(keys ...string)([]interface{}, error){
	var objs []interface{}
	objs, err := g_redis_cli.MGet(ctx, keys...).Result()
	if err != nil {
		return objs, err
	}
	return objs, nil
}


