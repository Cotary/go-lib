package mongo

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ---------------------------------------------------------------------------
// 纯函数单元测试（不依赖 MongoDB 实例）
// ---------------------------------------------------------------------------

func TestUnitBuildClientOpts_Defaults(t *testing.T) {
	cfg := &Config{URI: "mongodb://localhost:27017"}
	opts := buildClientOpts(cfg)
	assert.NotNil(t, opts)
}

func TestUnitBuildClientOpts_CustomValues(t *testing.T) {
	cfg := &Config{
		URI:            "mongodb://localhost:27017",
		AppName:        "test-app",
		MaxPoolSize:    200,
		MinPoolSize:    20,
		MaxConnIdleMs:  60000,
		ConnectTimeout: 5000,
	}
	opts := buildClientOpts(cfg)
	assert.NotNil(t, opts)
}

func TestUnitNewDB_InvalidURI(t *testing.T) {
	cfg := &Config{
		URI:            "mongodb://invalid-host-that-does-not-exist:99999",
		Database:       "test",
		ConnectTimeout: 1000,
	}
	_, err := NewDB(cfg)
	assert.Error(t, err)
}

func TestUnitNewClient_InvalidURI(t *testing.T) {
	cfg := &Config{
		URI:            "mongodb://invalid-host-that-does-not-exist:99999",
		Database:       "test",
		ConnectTimeout: 1000,
	}
	_, err := NewClient(cfg)
	assert.Error(t, err)
}

func TestUnitMustNewDB_Panics(t *testing.T) {
	cfg := &Config{
		URI:            "mongodb://invalid-host-that-does-not-exist:99999",
		Database:       "test",
		ConnectTimeout: 1000,
	}
	assert.Panics(t, func() {
		MustNewDB(cfg)
	})
}

func TestUnitMustNewClient_Panics(t *testing.T) {
	cfg := &Config{
		URI:            "mongodb://invalid-host-that-does-not-exist:99999",
		Database:       "test",
		ConnectTimeout: 1000,
	}
	assert.Panics(t, func() {
		MustNewClient(cfg)
	})
}

// ---------------------------------------------------------------------------
// 集成测试（需要真实 MongoDB，通过环境变量 MONGO_TEST_URI 控制）
//
// 运行方式：
//   MONGO_TEST_URI=mongodb://localhost:27017 go test -v -run TestIntegration ./dao/mongo/
//
// 事务测试需要副本集：
//   MONGO_TEST_URI=mongodb://localhost:27017/?replicaSet=rs0 go test -v -run TestIntegrationTransaction ./dao/mongo/
// ---------------------------------------------------------------------------

const testDB = "go_lib_test"

func skipIfNoMongo(t *testing.T) string {
	t.Helper()
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		t.Skip("skipping: set MONGO_TEST_URI to run integration tests")
	}
	return uri
}

func newTestDB(t *testing.T) *DB {
	t.Helper()
	uri := skipIfNoMongo(t)
	cfg := &Config{
		URI:      uri,
		Database: testDB,
		AppName:  "go-lib-test",
	}
	db, err := NewDB(cfg)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = db.Database.Drop(context.Background())
		_ = db.Close(context.Background())
	})
	return db
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	uri := skipIfNoMongo(t)
	cfg := &Config{
		URI:      uri,
		Database: testDB,
		AppName:  "go-lib-test",
	}
	client, err := NewClient(cfg)
	require.NoError(t, err)

	t.Cleanup(func() {
		db := client.Database()
		_ = db.Drop(context.Background())
		_ = client.Close(context.Background())
	})
	return client
}

// --- DB 模式集成测试 ---

func TestIntegrationNewDB(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	err := db.Client().Ping(ctx, nil)
	require.NoError(t, err)
}

func TestIntegrationInsertAndFind(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	coll := db.Collection("test_crud")

	doc := bson.D{
		{Key: "name", Value: "Alice"},
		{Key: "age", Value: 30},
	}
	_, err := coll.InsertOne(ctx, doc)
	require.NoError(t, err)

	var result bson.M
	err = coll.FindOne(ctx, bson.D{{Key: "name", Value: "Alice"}}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "Alice", result["name"])
	assert.Equal(t, int32(30), result["age"])
}

func TestIntegrationInsertMany(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	coll := db.Collection("test_insert_many")

	docs := []bson.D{
		{{Key: "name", Value: "Bob"}, {Key: "age", Value: 25}},
		{{Key: "name", Value: "Charlie"}, {Key: "age", Value: 35}},
		{{Key: "name", Value: "Diana"}, {Key: "age", Value: 28}},
	}
	res, err := coll.InsertMany(ctx, docs)
	require.NoError(t, err)
	assert.Len(t, res.InsertedIDs, 3)

	count, err := coll.CountDocuments(ctx, bson.D{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestIntegrationUpdate(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	coll := db.Collection("test_update")

	_, err := coll.InsertOne(ctx, bson.D{
		{Key: "name", Value: "Eve"},
		{Key: "age", Value: 22},
	})
	require.NoError(t, err)

	res, err := coll.UpdateOne(ctx,
		bson.D{{Key: "name", Value: "Eve"}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "age", Value: 23}}}},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(1), res.ModifiedCount)

	var result bson.M
	err = coll.FindOne(ctx, bson.D{{Key: "name", Value: "Eve"}}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, int32(23), result["age"])
}

func TestIntegrationDelete(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	coll := db.Collection("test_delete")

	docs := []bson.D{
		{{Key: "name", Value: "Frank"}, {Key: "group", Value: "A"}},
		{{Key: "name", Value: "Grace"}, {Key: "group", Value: "A"}},
		{{Key: "name", Value: "Hank"}, {Key: "group", Value: "B"}},
	}
	_, err := coll.InsertMany(ctx, docs)
	require.NoError(t, err)

	res, err := coll.DeleteMany(ctx, bson.D{{Key: "group", Value: "A"}})
	require.NoError(t, err)
	assert.Equal(t, int64(2), res.DeletedCount)

	count, err := coll.CountDocuments(ctx, bson.D{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestIntegrationFindWithFilter(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	coll := db.Collection("test_find_filter")

	docs := []bson.D{
		{{Key: "name", Value: "A"}, {Key: "score", Value: 90}},
		{{Key: "name", Value: "B"}, {Key: "score", Value: 60}},
		{{Key: "name", Value: "C"}, {Key: "score", Value: 85}},
		{{Key: "name", Value: "D"}, {Key: "score", Value: 45}},
	}
	_, err := coll.InsertMany(ctx, docs)
	require.NoError(t, err)

	cursor, err := coll.Find(ctx, bson.D{
		{Key: "score", Value: bson.D{{Key: "$gte", Value: 80}}},
	})
	require.NoError(t, err)

	var results []bson.M
	err = cursor.All(ctx, &results)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestIntegrationContextTimeout(t *testing.T) {
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)

	coll := db.Collection("test_timeout")
	_, err := coll.InsertOne(ctx, bson.D{{Key: "key", Value: "value"}})
	assert.Error(t, err)
}

// --- Client 模式集成测试 ---

func TestIntegrationClientDatabase(t *testing.T) {
	client := newTestClient(t)

	db := client.Database()
	assert.NotNil(t, db)
	assert.Equal(t, testDB, db.Name())

	otherDB := client.Database("other_test_db")
	assert.Equal(t, "other_test_db", otherDB.Name())
	_ = otherDB.Drop(context.Background())
}

func TestIntegrationClientCRUD(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()
	coll := client.Database().Collection("test_client_crud")

	_, err := coll.InsertOne(ctx, bson.D{{Key: "key", Value: "value"}})
	require.NoError(t, err)

	var result bson.M
	err = coll.FindOne(ctx, bson.D{{Key: "key", Value: "value"}}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "value", result["key"])
}

// --- 事务测试（需要副本集） ---

func TestIntegrationTransaction(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	result, err := client.Transaction(ctx, func(ctx context.Context) (any, error) {
		coll := client.Database().Collection("test_tx")

		_, err := coll.InsertOne(ctx, bson.D{
			{Key: "item", Value: "phone"},
			{Key: "qty", Value: 5},
		})
		if err != nil {
			return nil, err
		}
		_, err = coll.InsertOne(ctx, bson.D{
			{Key: "item", Value: "laptop"},
			{Key: "qty", Value: 2},
		})
		if err != nil {
			return nil, err
		}
		return "inserted 2 items", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "inserted 2 items", result)

	coll := client.Database().Collection("test_tx")
	count, err := coll.CountDocuments(ctx, bson.D{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestIntegrationTransactionRollback(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	coll := client.Database().Collection("test_tx_rollback")
	_, err := coll.InsertOne(ctx, bson.D{{Key: "item", Value: "existing"}})
	require.NoError(t, err)

	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "item", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	require.NoError(t, err)

	_, err = client.Transaction(ctx, func(ctx context.Context) (any, error) {
		_, err := coll.InsertOne(ctx, bson.D{{Key: "item", Value: "new_item"}})
		if err != nil {
			return nil, err
		}
		_, err = coll.InsertOne(ctx, bson.D{{Key: "item", Value: "existing"}})
		return nil, err
	})
	assert.Error(t, err)

	count, err := coll.CountDocuments(ctx, bson.D{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

// --- Close 测试 ---

func TestIntegrationDBClose(t *testing.T) {
	uri := skipIfNoMongo(t)
	cfg := &Config{URI: uri, Database: testDB}

	db, err := NewDB(cfg)
	require.NoError(t, err)

	err = db.Close(context.Background())
	assert.NoError(t, err)
}

func TestIntegrationClientClose(t *testing.T) {
	uri := skipIfNoMongo(t)
	cfg := &Config{URI: uri, Database: testDB}

	client, err := NewClient(cfg)
	require.NoError(t, err)

	err = client.Close(context.Background())
	assert.NoError(t, err)
}
