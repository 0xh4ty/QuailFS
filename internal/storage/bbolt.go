package storage

import (
	"errors"
	"go.etcd.io/bbolt"
	"os"
	"path/filepath"
)

func InitializeDatabase() (*bbolt.DB, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	file := filepath.Join(homeDir, ".quailfs-data", "quailfs.db")

	db, err := bbolt.Open(file, 0600, nil)
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("inventory"))
		if err != nil {
			return err
		}

		_, err = tx.CreateBucketIfNotExists([]byte("catalogs"))
		if err != nil {
			return err
		}

		_, err = tx.CreateBucketIfNotExists([]byte("ads"))
		if err != nil {
			return err
		}

		_, err = tx.CreateBucketIfNotExists([]byte("peers"))
		if err != nil {
			return err
		}

		_, err = tx.CreateBucketIfNotExists([]byte("claims"))
		if err != nil {
			return err
		}

		_, err = tx.CreateBucketIfNotExists([]byte("bootstrap"))
		if err != nil {
			return err
		}

		_, err = tx.CreateBucketIfNotExists([]byte("node_private_key"))
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func Put(db *bbolt.DB, bucketName string, key []byte, value []byte) error {
	err := db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return errors.New("bucket not found")
		}
		return bucket.Put(key, value)
	})
	if err != nil {
		return err
	}
	return nil
}

func Get(db *bbolt.DB, bucketName string, key []byte) ([]byte, error) {
	var value []byte
	err := db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return errors.New("bucket not found")
		}

		value = append([]byte(nil), bucket.Get(key)...)

		if value == nil {
			return nil
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return value, nil
}

func Delete(db *bbolt.DB, bucketName string, key []byte) error {
	return db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return errors.New("bucket not found")
		}

		return bucket.Delete(key)
	})
}
