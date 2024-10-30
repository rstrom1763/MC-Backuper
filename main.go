package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Create the DB connection and create the tables if they don't already exist
func initDB(path string) *sql.DB {

	createTablesQuery := `CREATE TABLE IF NOT EXISTS instances (
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

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		log.Fatal(fmt.Sprintf("Could not open DB: %s", err))
	}
	err = db.Ping()
	if err != nil {
		log.Fatal(fmt.Sprintf("Could not ping DB: %s", err))
	}

	_, err = db.Exec(createTablesQuery)
	if err != nil {
		log.Fatalf("Could not create tables: %s", err)
	}

	return db

}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	if err == nil {
		return true // File exists
	}
	if os.IsNotExist(err) {
		return false // File does not exist
	}
	return false // Error occurred (e.g., permission denied)
}

// runCommand takes a command string, executes it, and returns the output or an error
func runCommand(command string) (string, error) {
	// Split the command string into command name and arguments
	parts := strings.Fields(command)
	cmd := exec.Command(parts[0], parts[1:]...)

	// Run the command and capture the output
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf(string(output))
	}

	// Return the output as a string
	return string(output), nil
}

func runDockerCommand(command string, container string) (string, error) {
	output, err := runCommand(fmt.Sprintf("/usr/bin/docker exec %s rcon-cli %s", container, command))
	if err != nil {
		return "", fmt.Errorf("failed to run docker command: %v, error: %v", command, err)
	}

	return output, nil
}

func getNumberOfPlayers(container string) (int32, error) {
	output, err := runDockerCommand("/list", container)
	if err != nil {
		return -1, err
	}

	number, err := strconv.Atoi(strings.Split(output, " ")[2])
	if err != nil {
		return -1, err
	}

	return int32(number), nil
}

func say(input string, container string) error {
	_, err := runDockerCommand(fmt.Sprintf("/say %v", input), container)
	if err != nil {
		return err
	}
	return nil
}

// checkAWSCLI checks if the AWS CLI is installed and configured
func checkAWSCLI() error {

	// Check if AWS CLI is installed
	if !fileExists("/usr/bin/aws") {
		return fmt.Errorf("AWS CLI is not installed or not found in /usr/bin")
	}
	return nil
}

// Returns the time as a string in the desired format
func getTime() string {
	currentTime := time.Now()
	formattedTime := fmt.Sprint(currentTime.Format("2006-01-02_15:04:05"))
	formattedTime = strings.Replace(formattedTime, ":", "_", -1)
	return formattedTime
}

// Storage class options:
// STANDARD
// INTELLIGENT_TIERING
// STANDARD_IA
// ONEZONE_IA
// GLACIER
// DEEP_ARCHIVE
// REDUCED_REDUNDANCY

// Backs up the file to the S3 bucket
func backUpToS3(fileName string, bucket string, prefix string, storageClass string) error {

	s3Path := fmt.Sprintf("s3://%v/%v", bucket, prefix)

	_, err := runCommand(fmt.Sprintf("aws s3 cp %v %v/%v --storage-class %v", fileName, s3Path, fileName, storageClass))
	if err != nil {
		return err
	}
	return nil

}

func deleteFile(filePath string) error {
	// Attempt to remove the file
	err := os.Remove(filePath)
	if err != nil {
		return err
	}
	return nil
}

func backupInstance(db *sql.DB, instance Instance) error {

	transaction, err := db.Begin()
	if err != nil {
		return fmt.Errorf("Could not start transaction: %s", err)
	}

	// If the function errors out, call rollback.
	// If everything is successful and tx is committed, rollback should have no effect
	defer func(transaction *sql.Tx) {
		_ = transaction.Rollback()
	}(transaction)

	err = os.Chdir(instance.workingPath)
	if err != nil {
		return fmt.Errorf("Could not change working directory: %s", err)
	}

	// Disable command output
	// This is so there isn't a ton of output to the console all the time
	output, err := runDockerCommand("/gamerule sendCommandFeedback false", instance.containerName)
	if err != nil {
		return fmt.Errorf("Could not disable command feedback: %v, error: %v", output, err)
	}

	var currentTime string
	var tarFileName string
	var playerCount int32

	currentTime = getTime()
	tarFileName = fmt.Sprintf("world%v.tar.gz", currentTime)

	// Check if there are players online
	// We don't want to save if there aren't even any players playing
	playerCount, err = getNumberOfPlayers(instance.containerName)
	if err != nil {
		return fmt.Errorf("Could not get playerCount of players: %v", err)
	}

	// If there are no players, wait the wait interval, else print the saving message
	if playerCount == 0 {
		fmt.Printf("%v: No players online, skipping...\n", instance.containerName)
		return nil
	} else if playerCount == 1 {
		fmt.Printf("%v: There is %d player online, saving...\n", instance.containerName, playerCount)
	} else {
		fmt.Printf("%v: There are %d players online, saving...\n", instance.containerName, playerCount)
	}

	// Save the mc world
	_ = say("Saving world...", instance.containerName) // Tell players that the world is saving
	output, err = runDockerCommand("/save-all", instance.containerName)
	if err != nil {
		_ = say("Failed to save world", instance.containerName)
		return fmt.Errorf("Could not save world: %v", err)
	}

	// Buffer time to let things save
	time.Sleep(10 * time.Second)

	// Disable saving
	// This ensures the save file doesn't change during the copy
	output, err = runDockerCommand("/save-off", instance.containerName)
	if err != nil {
		return fmt.Errorf("Could not save world: %v", err)
	}

	// Buffer to make sure the files aren't being accessed anymore
	time.Sleep(5 * time.Second)

	// Tar the world
	// If it fails due to a changed during access, try again until it works
	for {
		output, err = runCommand(fmt.Sprintf("/bin/tar -czf ./%v ./%v", tarFileName, instance.dirName))
		if err != nil {
			log.Printf("Could not compress world: %v, error: %v\n", output, err)

			err = deleteFile(tarFileName)
			if err != nil {
				return fmt.Errorf("Could not delete file: %v", err)
			}

			time.Sleep(5 * time.Second) // Time buffer to hopefully allow whatever happened to clear up
			continue
		}
		break
	}

	var storageClass = "STANDARD" // Storage class used for the S3 storage

	// Upload the save to S3
	err = backUpToS3(tarFileName, instance.s3Bucket, instance.prefix, storageClass)
	if err != nil {
		return fmt.Errorf("Could not backup to S3: %v", err)
	}

	tarFileStats, err := os.Stat(tarFileName)
	if err != nil {
		return fmt.Errorf("Could not stat tar file: %v", err)
	}

	_, err = transaction.Exec("INSERT INTO saves (filename,size,instance_id) VALUES (?,?,?)", tarFileName, tarFileStats.Size(), instance.id)
	if err != nil {
		return fmt.Errorf("Could not insert save record: %v", err)
	}

	// Delete the tar file
	err = deleteFile(tarFileName)
	if err != nil {
		return fmt.Errorf("Could not delete tar file: %v", err)
	}

	// Re-enable saving
	output, err = runDockerCommand("/save-on", instance.containerName)
	if err != nil {
		return fmt.Errorf("Could not re-enable mc saving: %v, error: %v", output, err)
	}

	_ = say("Save successful!", instance.containerName)
	fmt.Printf("%v: Save success!\n", instance.containerName)

	err = transaction.Commit()
	if err != nil {
		return fmt.Errorf("Could not commit transaction: %v", err)
	}
	return nil

}

func getInstances(db *sql.DB) ([]Instance, error) {

	var containerName, description, dirName, s3Bucket, prefix, workingPath string
	var keepInventory, active bool
	var instances []Instance
	var id int

	rows, err := db.Query("SELECT id,container_name,description,dir_name,s3_bucket,prefix,working_path,active,keep_inventory FROM instances")
	if err != nil {
		log.Fatalf("Could not query DB: %s", err)
	}

	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Printf("Error closing rows: %s", err)
		}
	}(rows)

	for rows.Next() {
		err = rows.Scan(&id, &containerName, &description, &dirName, &s3Bucket, &prefix, &workingPath, &active, &keepInventory)
		if err != nil {
			return nil, fmt.Errorf("Error scanning row: %s", err)
		}

		// Append the instance to the instances slice
		instances = append(instances, Instance{
			id:            id,
			containerName: containerName,
			description:   description,
			dirName:       dirName,
			s3Bucket:      s3Bucket,
			prefix:        prefix,
			workingPath:   workingPath,
			active:        active,
			keepInventory: keepInventory,
		})

	}
	return instances, nil
}

type Instance struct {
	id            int
	containerName string
	description   string
	dirName       string
	keepInventory bool
	prefix        string
	s3Bucket      string
	active        bool
	workingPath   string
}

func main() {

	var saveInterval int32 = 30 // 30 minutes by default
	waitDuration := time.Duration(saveInterval) * time.Minute
	dbPath := "./db.sqlite" // The path to the sqlite file

	// Make sure AWS CLI is installed and configured
	err := checkAWSCLI()
	if err != nil {
		log.Fatalf(err.Error())
	}

	db := initDB(dbPath)

	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {
			log.Fatal(fmt.Sprintf("Could not close DB: %s", err))
		}
	}(db)

	// An example of an insert for a new instance into the database
	/*
		_, err = db.Exec("INSERT INTO instances (container_name,description,dir_name,s3_bucket,prefix,working_path,keep_inventory) VALUES (?,?,?,?,?,?,?)",
			"test-container", "Description of world", "world", "bucket-name", "prefix-to-upload-to", "/home/example/folder", true)
		if err != nil {
			log.Fatalf("Could not insert into DB: %s", err)
		}
	*/

	for {
		instances, err := getInstances(db)
		if err != nil {
			log.Fatalf("Could not get instances: %s", err)
		}

		for _, instance := range instances {

			if instance.active == false {
				continue
			}

			// Set the keepInventory setting based on the that field in the instance
			if instance.keepInventory == true {
				_, _ = runDockerCommand("/gamerule keepInventory true", instance.containerName)
			} else {
				_, _ = runDockerCommand("/gamerule keepInventory false", instance.containerName)
			}

			// Begin the actual backup of the instance
			err = backupInstance(db, instance)
			if err != nil {
				fmt.Printf("Could not backup the instance: %v", err)
			}

		}

		time.Sleep(waitDuration)
	}

}
