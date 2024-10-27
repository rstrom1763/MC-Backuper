package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
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

func main() {
	containerName := "mc"
	var saveInterval int32 = 30       // In minutes
	s3Bucket := "ryans-backup-bucket" // S3 bucket to backup to
	prefix := "minecraft/erik_new_world"
	dirName := "world"
	workingPath := "/home/ryan/erik_mc_world"
	storageClass := "STANDARD"

	/*
		containerName := "sammie_mc"
		var saveInterval int32 = 30       // In minutes
		s3Bucket := "ryans-backup-bucket" // S3 bucket to backup to
		prefix := "minecraft/sammie"
		dirName := "world"
		workingPath := "/home/ryan/sammie_mc"
		storageClass := "STANDARD"
	*/

	waitDuration := time.Duration(saveInterval) * time.Minute

	err := os.Chdir(workingPath)
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Make sure AWS CLI is installed and configured
	err = checkAWSCLI()
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Disable command output
	// This is so there isn't a ton of output to the console all the time
	output, err := runDockerCommand("gamerule sendCommandFeedback false", containerName)
	if err != nil {
		log.Fatalf("Could not disable command feedback: %v, error: %v", output, err)
	}

	var currentTime string
	var tarFileName string
	var number int32

	for {

		currentTime = getTime()
		tarFileName = fmt.Sprintf("world%v.tar.gz", currentTime)

		// Check if there are players online
		// We don't want to save if there aren't even any players playing
		number, err = getNumberOfPlayers(containerName)
		if err != nil {
			log.Fatalf("Could not get number of players: %v", err)
		}

		if number == 0 {
			fmt.Println("No players online, skipping...")
			time.Sleep(waitDuration)
			continue
		} else if number == 1 {
			fmt.Printf("There is %d player online, saving...\n", number)
		} else {
			fmt.Printf("There are %d players online, saving...\n", number)
		}

		// Save the mc world
		_ = say("Saving world...", containerName)
		output, err := runDockerCommand("/save-all", containerName)
		if err != nil {
			_ = say("Failed to save world", containerName)
			log.Fatalf("Could not save mc world: %v, error: %v", output, err)
		}

		// Disable saving
		// This ensures the save file doesn't change during the copy
		output, err = runDockerCommand("/save-off", containerName)
		if err != nil {
			log.Fatalf("Could not disable mc saving: %v, error: %v", output, err)
		}

		// Buffer to make sure the files aren't being accessed anymore
		time.Sleep(5 * time.Second)

		// Tar the world
		// If it fails due to a changed during access, try again until it works
		for {
			output, err = runCommand(fmt.Sprintf("/bin/tar -czf ./%v ./%v", tarFileName, dirName))
			if err != nil {
				log.Printf("Could not compress world: %v, error: %v\n", output, err)

				err = deleteFile(tarFileName)
				if err != nil {
					log.Fatalf(err.Error())
				}

				time.Sleep(5 * time.Second) // Time buffer to hopefully allow whatever happened to clear up
				continue
			}
			break
		}

		// Upload the save to S3
		err = backUpToS3(tarFileName, s3Bucket, prefix, storageClass)
		if err != nil {
			log.Fatalf(err.Error())
		}

		// Delete the tar file
		err = deleteFile(tarFileName)
		if err != nil {
			log.Fatalf(err.Error())
		}

		// Re-enable saving
		output, err = runDockerCommand("/save-on", containerName)
		if err != nil {
			log.Fatalf("Could not re-enable mc saving: %v, error: %v", output, err)
		}

		_ = say("Save successful!", containerName)

		// Sleep until the next save interval
		time.Sleep(waitDuration)

	}

}
