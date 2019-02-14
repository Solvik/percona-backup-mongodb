package client

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/kr/pretty"
	"github.com/percona/percona-backup-mongodb/internal/testutils"
	pb "github.com/percona/percona-backup-mongodb/proto/messages"
	log "github.com/sirupsen/logrus"
)

func TestValidateFilesystemStorage(t *testing.T) {
	input, err := buildInputParams()
	if err != nil {
		t.Fatalf("Cannot build agent's input params: %s", err)
	}
	c, err := NewClient(context.TODO(), input)
	msg := &pb.GetStorageInfo{
		StorageName: "local-filesystem",
	}

	stg, err := input.Storages.Get("local-filesystem")
	if err != nil {
		t.Fatalf("Cannot get S3 storage: %s", err)
	}

	wantFs := pb.ClientMessage{
		Version:  0,
		ClientId: "",
		Payload: &pb.ClientMessage_StorageInfo{
			StorageInfo: &pb.StorageInfo{
				Name:     "local-filesystem",
				Type:     "filesystem",
				Valid:    true,
				CanRead:  true,
				CanWrite: true,
				S3:       &pb.S3{},
				Filesystem: &pb.Filesystem{
					Path: stg.Filesystem.Path,
				},
			},
		},
	}

	info, err := c.processGetStorageInfo(msg)
	if err != nil {
		t.Errorf("Cannot process GetStorageInfo: %s", err)
	}
	if !reflect.DeepEqual(wantFs, info) {
		t.Errorf("Invalid info.\nWant:\n%s\nGot:\n%s", pretty.Sprint(wantFs), pretty.Sprint(info))
	}
}

func TestValidateS3Storage(t *testing.T) {
	input, err := buildInputParams()
	if err != nil {
		t.Fatalf("Cannot build agent's input params: %s", err)
	}
	c, err := NewClient(context.TODO(), input)
	msg := &pb.GetStorageInfo{
		StorageName: "s3-us-west",
	}

	stg, err := input.Storages.Get("s3-us-west")
	if err != nil {
		t.Fatalf("Cannot get S3 storage: %s", err)
	}

	wantFs := pb.ClientMessage{
		Version:  0,
		ClientId: "",
		Payload: &pb.ClientMessage_StorageInfo{
			StorageInfo: &pb.StorageInfo{
				Name:     "s3-us-west",
				Type:     "s3",
				Valid:    true,
				CanRead:  true,
				CanWrite: true,
				S3: &pb.S3{
					Region:      "us-west-2",
					EndpointUrl: "",
					Bucket:      stg.S3.Bucket,
				},
				Filesystem: &pb.Filesystem{},
			},
		},
	}

	info, err := c.processGetStorageInfo(msg)
	if err != nil {
		t.Errorf("Cannot process GetStorageInfo: %s", err)
	}
	if !reflect.DeepEqual(wantFs, info) {
		t.Errorf("Invalid info.\nWant:\n%s\nGot:\n%s", pretty.Sprint(wantFs), pretty.Sprint(info))
	}
}

func TestFsBackupAndRestore(t *testing.T) {
	dbName := "test001"
	col1 := "col1"
	col2 := "col2"
	ndocs := 100000
	bulkSize := 5000

	input, err := buildInputParams()
	if err != nil {
		t.Fatalf("Cannot build agent's input params: %s", err)
	}

	c, err := NewClient(context.TODO(), input)
	if err != nil {
		t.Fatalf("Cannot get S3 storage: %s", err)
	}
	c.dbConnect()

	session, err := mgo.DialWithInfo(testutils.PrimaryDialInfo(t, testutils.MongoDBShard1ReplsetName))
	if err != nil {
		log.Fatalf("Cannot connect to the DB: %s", err)
	}

	session.SetMode(mgo.Strong, true)
	session.DB(dbName).C(col1).DropCollection()
	session.DB(dbName).C(col2).DropCollection()

	generateDataToBackup(t, c.mdbSession, dbName, col1, ndocs, bulkSize)
	generateDataToBackup(t, c.mdbSession, dbName, col2, ndocs, bulkSize)

	msg := &pb.StartBackup{
		BackupType:      pb.BackupType_BACKUP_TYPE_LOGICAL,
		DbBackupName:    "0001.dump",
		OplogBackupName: "",
		CompressionType: pb.CompressionType_COMPRESSION_TYPE_NO_COMPRESSION,
		Cypher:          pb.Cypher_CYPHER_NO_CYPHER,
		OplogStartTime:  0,
		Description:     "test001",
		StorageName:     "local-filesystem",
	}

	err = c.runDBBackup(msg)
	if err != nil {
		t.Errorf("Cannot process restore from s3: %s", err)
	}

	rmsg := &pb.RestoreBackup{
		MongodbHost: "127.0.0.1",
		BackupType:  pb.BackupType_BACKUP_TYPE_LOGICAL,
		//SourceBucket
		DbSourceName:      "0001.dump",
		OplogSourceName:   "",
		CompressionType:   pb.CompressionType_COMPRESSION_TYPE_NO_COMPRESSION,
		Cypher:            pb.Cypher_CYPHER_NO_CYPHER,
		OplogStartTime:    0,
		SkipUsersAndRoles: true,
		Host:              "127.0.0.1",
		Port:              "17001",
		StorageName:       "local-filesystem",
	}

	err = c.restoreDBDump(rmsg)
	if err != nil {
		t.Errorf("Cannot process restore from s3: %s", err)
	}

	if err := testutils.CleanTempDirAndBucket(); err != nil {
		t.Errorf("Cannot clean up directory and bucket: %s", err)
	}
}

func TestS3sBackupAndRestore(t *testing.T) {
	dbName := "test001"
	col1 := "col1"
	col2 := "col2"
	ndocs := 100000
	bulkSize := 5000

	input, err := buildInputParams()
	if err != nil {
		t.Fatalf("Cannot build agent's input params: %s", err)
	}

	c, err := NewClient(context.TODO(), input)
	if err != nil {
		t.Fatalf("Cannot get S3 storage: %s", err)
	}
	c.dbConnect()

	session, err := mgo.DialWithInfo(testutils.PrimaryDialInfo(t, testutils.MongoDBShard1ReplsetName))
	if err != nil {
		log.Fatalf("Cannot connect to the DB: %s", err)
	}

	session.SetMode(mgo.Strong, true)
	session.DB(dbName).C(col1).DropCollection()
	session.DB(dbName).C(col2).DropCollection()

	generateDataToBackup(t, c.mdbSession, dbName, col1, ndocs, bulkSize)
	generateDataToBackup(t, c.mdbSession, dbName, col2, ndocs, bulkSize)

	msg := &pb.StartBackup{
		BackupType:      pb.BackupType_BACKUP_TYPE_LOGICAL,
		NamePrefix:      "backup_test_",
		DbBackupName:    "0001.dump",
		OplogBackupName: "",
		CompressionType: pb.CompressionType_COMPRESSION_TYPE_NO_COMPRESSION,
		Cypher:          pb.Cypher_CYPHER_NO_CYPHER,
		OplogStartTime:  0,
		Description:     "test001",
		StorageName:     "s3-us-west",
	}

	err = c.runDBBackup(msg)
	if err != nil {
		t.Errorf("Cannot process restore from s3: %s", err)
	}

	rmsg := &pb.RestoreBackup{
		MongodbHost: "127.0.0.1",
		BackupType:  pb.BackupType_BACKUP_TYPE_LOGICAL,
		//SourceBucket
		DbSourceName:      "0001.dump",
		OplogSourceName:   "",
		CompressionType:   pb.CompressionType_COMPRESSION_TYPE_NO_COMPRESSION,
		Cypher:            pb.Cypher_CYPHER_NO_CYPHER,
		OplogStartTime:    0,
		SkipUsersAndRoles: true,
		Host:              "127.0.0.1",
		Port:              "17001",
		StorageName:       "s3-us-west",
	}

	err = c.restoreDBDump(rmsg)
	if err != nil {
		t.Errorf("Cannot process restore from s3: %s", err)
	}

	if err := testutils.CleanTempDirAndBucket(); err != nil {
		t.Errorf("Cannot clean up directory and bucket: %s", err)
	}
}

func buildInputParams() (InputOptions, error) {
	port := testutils.MongoDBShard1PrimaryPort
	rs := testutils.MongoDBShard1ReplsetName

	di, err := testutils.DialInfoForPort(rs, port)
	if err != nil {
		return InputOptions{}, err
	}

	storages := testutils.TestingStorages()

	dbConnOpts := ConnectionOptions{
		Host:           testutils.MongoDBHost,
		Port:           port,
		User:           di.Username,
		Password:       di.Password,
		ReplicasetName: di.ReplicaSetName,
	}

	input := InputOptions{
		BackupDir:     os.TempDir(),
		DbConnOptions: dbConnOpts,
		GrpcConn:      nil,
		Logger:        nil,
		Storages:      storages,
	}

	return input, nil
}

func generateDataToBackup(t *testing.T, session *mgo.Session, dbName string, colName string, ndocs, bulkSize int) {
	// Don't check for error because the collection might not exist.
	session.DB(dbName).C(colName).DropCollection()
	session.DB(dbName).C(colName).EnsureIndexKey("number")
	session.Refresh()

	number := 0
	for i := 0; i < ndocs/bulkSize; i++ {
		docs := make([]interface{}, 0, bulkSize)
		bulk := session.DB(dbName).C(colName).Bulk()
		for j := 0; j < bulkSize; j++ {
			number++
			docs = append(docs, bson.M{"number": number})
		}
		bulk.Insert(docs...)
		_, err := bulk.Run()
		if err != nil {
			t.Fatalf("Cannot insert data to back up: %s", err)
		}
	}
}
