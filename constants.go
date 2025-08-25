package main

const INITDB_QUERY string = `CREATE TABLE IF NOT EXISTS instances (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		container_name varchar(255) NOT NULL UNIQUE,
		description text,
		dir_name text NOT NULL,
		keep_inventory boolean NOT NULL,
		s3_bucket VARCHAR(255) NOT NULL,
		prefix TEXT NOT NULL,
		working_path TEXT NOT NULL,
		active BOOLEAN DEFAULT TRUE NOT NULL,
		created_at BIGINT DEFAULT CURRENT_TIMESTAMP
	);
	
	CREATE TABLE IF NOT EXISTS saves (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		filename VARCHAR(255) NOT NULL,
		deleted BOOLEAN NOT NULL DEFAULT FALSE,
		size BIGINT NOT NULL,
		created_at BIGINT DEFAULT CURRENT_TIMESTAMP,
		instance_id INT NOT NULL,
		FOREIGN KEY (instance_id) REFERENCES instances(id)
	);`

const SAVE_INTERVAL_MINUTES int = 30
const SAVE_RETENTION_COUNT int = 5         // How many saves that should be held on to at any given point for each instance
const DB_PATH string = "./db.sqlite"       // Path to the sqlite file
const S3_STORAGE_CLASS string = "STANDARD" // Storage class used for the S3 storage
const LOG_FILE_PATH string = "./log.log"
