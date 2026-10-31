package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/rstrom1763/MC-Backuper/client"
	"github.com/rstrom1763/MC-Backuper/daos"
)

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
		return "", errors.New(string(output))
	}

	// Return the output as a string
	return string(output), nil
}

func runMinecraftCommand(dockerClient client.DockerClient, command string, containerName string) (string, error) {

	output, err := dockerClient.RunCommand(context.Background(), containerName, command)
	if err != nil {
		return "", fmt.Errorf("could not run Docker command: %s", err.Error())
	}

	return output, nil
}

func getNumberOfPlayers(dockerClient client.DockerClient, container string) (int32, error) {
	output, err := runMinecraftCommand(dockerClient, "list", container)
	if err != nil {
		return -1, err
	}

	number, err := strconv.Atoi(strings.Split(output, " ")[2])
	if err != nil {
		return -1, err
	}

	return int32(number), nil
}

func say(dockerClient client.DockerClient, input string, container string) error {
	_, err := runMinecraftCommand(dockerClient, fmt.Sprintf("say %v", input), container)
	if err != nil {
		return err
	}
	return nil
}

// checkAWSCLI checks if the AWS CLI is installed and configured
func checkAWSCLI() error {

	// Check if AWS CLI is installed
	if !fileExists("/usr/local/bin/aws") {
		return fmt.Errorf("AWS CLI is not installed or not found in /usr/local/bin")
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

// Backs up the file to the S3 bucket
func backUpToS3(fileName string, bucket string, prefix string, storageClass string) error {

	s3Path := fmt.Sprintf("s3://%v/%v", bucket, prefix)

	_, err := runCommand(fmt.Sprintf("aws s3 cp %v %v/%v --storage-class %v", fileName, s3Path, fileName, storageClass))
	if err != nil {
		return err
	}
	return nil
}

func deleteS3File(fileName string, bucket string, prefix string) error {

	s3Path := fmt.Sprintf("s3://%v/%v/%v", bucket, prefix, fileName)

	_, err := runCommand(fmt.Sprintf("aws s3 rm %v", s3Path))
	if err != nil {
		return fmt.Errorf("could not delete save file in S3: %v", err)
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

func backupInstance(dockerClient client.DockerClient, dbDAO daos.DBDAO, instance daos.Instance) error {

	// If the function errors out or finishes, ensure saving and feedback are re-enabled
	defer func() {
		// Re-enable saving
		output, err := runMinecraftCommand(dockerClient, "/save-on", instance.ContainerName)
		if err != nil {
			log.Printf("Error: %v: could not reenable mc saving: %v, error: %v", instance.ContainerName, output, err)
		}

		// Re-enable command feedback
		output, err = runMinecraftCommand(dockerClient, "/gamerule sendCommandFeedback true", instance.ContainerName)
		if err != nil {
			log.Printf("Error: %v: could not reenable command feedback: %v, error: %v", instance.ContainerName, output, err)
		}
	}()

	err := os.Chdir(instance.WorkingPath)
	if err != nil {
		return fmt.Errorf("could not change working directory: %s", err)
	}

	// Disable command output
	// This is so there isn't a ton of output to the console all the time
	output, err := runMinecraftCommand(dockerClient, "/gamerule sendCommandFeedback false", instance.ContainerName)
	if err != nil {
		return fmt.Errorf("could not disable command feedback: %v, error: %v", output, err)
	}

	var currentTime string
	var tarFileName string

	currentTime = getTime()
	tarFileName = fmt.Sprintf("world%v.tar.gz", currentTime)

	// Save the mc world
	_ = say(dockerClient, "Saving world...", instance.ContainerName) // Tell players that the world is saving

	// Disable saving
	// This ensures the save file doesn't change during the copy
	output, err = runMinecraftCommand(dockerClient, "/save-off", instance.ContainerName)
	if err != nil {
		return fmt.Errorf("could not save world: %v", err)
	}

	// Buffer time
	time.Sleep(5 * time.Second)

	output, err = runMinecraftCommand(dockerClient, "/save-all", instance.ContainerName)
	if err != nil {
		_ = say(dockerClient, "Failed to save world", instance.ContainerName)
		return fmt.Errorf("could not save world: %v", err)
	}

	// Buffer time to let things save
	time.Sleep(10 * time.Second)

	// Tar the world
	// If it fails due to a change during access, try again until it works
	for {
		output, err = runCommand(fmt.Sprintf("/bin/tar -czf ./%v ./%v", tarFileName, instance.DirName))
		if err != nil {

			// Make sure the error doesn't have a newline character
			err = errors.New(strings.Replace(err.Error(), "\n", "", -1))

			log.Printf("Error: %v: Could not compress world, error: %v\n", output, err)

			err = deleteFile(tarFileName)
			if err != nil {
				return fmt.Errorf("could not delete file: %v", err)
			}

			time.Sleep(5 * time.Second) // Time buffer to hopefully allow whatever happened to clear up
			continue
		}
		break
	}

	var storageClass = "STANDARD" // Storage class used for the S3 storage

	// Upload the save to S3
	err = backUpToS3(tarFileName, instance.S3Bucket, instance.Prefix, storageClass)
	if err != nil {
		return fmt.Errorf("could not backup to S3: %v", err)
	}

	// Grabs info about the file. We are interested in the size of the file
	tarFileStats, err := os.Stat(tarFileName)
	if err != nil {
		return fmt.Errorf("could not stat tar file: %v", err)
	}

	// Add the save to the DB
	err = dbDAO.AddSaveRecord(context.Background(), instance.ID, tarFileName, tarFileStats.Size())
	if err != nil {
		return fmt.Errorf("could not insert save record: %v", err)
	}

	// Delete the tar file
	err = deleteFile(tarFileName)
	if err != nil {
		return fmt.Errorf("could not delete tar file: %v", err)
	}

	_ = say(dockerClient, "Save successful!", instance.ContainerName)
	log.Printf("Info: %v: Save success!\n", instance.ContainerName)

	return nil

}

func removeOldSaves(dbDAO daos.DBDAO, instance daos.Instance, saveRetention int) error {

	saves, err := dbDAO.GetActiveSaves(context.Background(), instance.ID)
	if err != nil {
		return fmt.Errorf("could not query active saves: %v", err)
	}

	for i, save := range saves {
		if i < saveRetention {
			continue
		}

		err = deleteS3File(save.Filename, instance.S3Bucket, instance.Prefix)
		if err != nil {
			return fmt.Errorf("could not delete save file: %v", err)
		}

		err = dbDAO.MarkSaveDeleted(context.Background(), save.ID)
		if err != nil {
			return fmt.Errorf("could not update save record: %v", err)
		}
	}

	return nil
}

func main() {

	// Open log file (create if not exists, append if exists)
	logFile, err := os.OpenFile("./log.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}

	// Create MultiWriter for stdout and file
	multi := io.MultiWriter(os.Stdout, logFile)

	// Set output for the default logger
	log.SetOutput(multi)
	log.Print("Info: Starting backup service\n")

	var saveInterval int32 = 30 // 30 minutes by default
	waitDuration := time.Duration(saveInterval) * time.Minute
	dbPath := "./db.sqlite" // The path to the sqlite file
	saveRetention := 5      // How many saves that should be held on to at any given point for each instance

	// Make sure AWS CLI is installed and configured
	err = checkAWSCLI()
	if err != nil {
		log.Fatal(err)
	}

	dbDAO, err := daos.NewSqliteDAO(dbPath)
	if err != nil {
		log.Fatalf("Could not open the database: %v", err)
	}

	defer func(dbDAO daos.DBDAO) {
		err := dbDAO.Close()
		if err != nil {
			log.Fatal(fmt.Sprintf("Could not close DB: %s", err))
		}
	}(dbDAO)

	dockerClient, err := client.NewDockerClient()
	if err != nil {
		log.Fatalf("Could not create Docker client: %v", err)
	}
	defer dockerClient.Close()

	for {
		instances, err := dbDAO.GetInstances(context.Background())
		if err != nil {
			log.Fatalf("Could not get instances: %s", err)
		}

		for _, instance := range instances {

			// If the instance is set to inactive, skip it
			if !instance.Active {
				continue
			}

			// See if the container is even running
			containerRunning, err := dockerClient.IsContainerRunning(context.Background(), instance.ContainerName)
			if err != nil {
				log.Fatalf("There was an error seeing if container: %v : was running: %v", instance.ContainerName, err)
			}
			if !containerRunning {
				log.Printf("Info: %v: Not running, skipping\n", instance.ContainerName)
				continue
			}

			var playerCount int32

			// Check if there are players online
			// We don't want to save if there aren't even any players playing
			playerCount, err = getNumberOfPlayers(dockerClient, instance.ContainerName)
			if err != nil {
				log.Printf("Error: %v: Could not get player count: %v", instance.ContainerName, err)
			}

			// If there are no players, wait the wait interval, else print the saving message
			if playerCount < 1 {
				log.Printf("Info: %v: No players online, skipping\n", instance.ContainerName)
				continue
			} else if playerCount == 1 {
				log.Printf("Info: %v: There is %d player online, saving\n", instance.ContainerName, playerCount)
			} else {
				log.Printf("Info: %v: There are %d players online, saving\n", instance.ContainerName, playerCount)
			}

			err = removeOldSaves(dbDAO, instance, saveRetention-1) // The minus one is to account for the save that is about to happen
			if err != nil {
				log.Printf("Error: %v: Could not remove old saves: %v", instance.ContainerName, err)
			}

			// Set the keepInventory setting based on the that field in the instance
			if instance.KeepInventory {
				_, _ = runMinecraftCommand(dockerClient, "/gamerule keepInventory true", instance.ContainerName)
			} else {
				_, _ = runMinecraftCommand(dockerClient, "/gamerule keepInventory false", instance.ContainerName)
			}

			// Begin the actual backup of the instance
			err = backupInstance(dockerClient, dbDAO, instance)
			if err != nil {
				log.Printf("Error: %v: Could not backup the instance: %v", instance.ContainerName, err)
			}

		}

		log.Printf("Info: Waiting for %v minutes\n", saveInterval)
		time.Sleep(waitDuration)
	}

}
