package packaging

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-flutter-desktop/hover/internal/build"
	"github.com/go-flutter-desktop/hover/internal/log"
	"github.com/go-flutter-desktop/hover/internal/pubspec"
)

var packagingPath = filepath.Join(build.BuildPath, "packaging")

func packagingFormatPath(packagingFormat string) string {
	directoryPath, err := filepath.Abs(filepath.Join(packagingPath, packagingFormat))
	if err != nil {
		log.Errorf("Failed to resolve absolute path for %s directory: %v", packagingFormat, err)
		os.Exit(1)
	}
	return directoryPath
}

func createPackagingFormatDirectory(packagingFormat string) {
	if _, err := os.Stat(packagingFormatPath(packagingFormat)); !os.IsNotExist(err) {
		log.Errorf("A file or directory named `%s` already exists. Cannot continue packaging init for %s.", packagingFormat, packagingFormat)
		os.Exit(1)
	}
	err := os.MkdirAll(packagingFormatPath(packagingFormat), 0775)
	if err != nil {
		log.Errorf("Failed to create %s directory %s: %v", packagingFormat, packagingFormatPath(packagingFormat), err)
		os.Exit(1)
	}
}

// AssertPackagingFormatInitialized exits hover if the requested
// packagingFormat isn't initialized.
func AssertPackagingFormatInitialized(packagingFormat string) {
	if _, err := os.Stat(packagingFormatPath(packagingFormat)); os.IsNotExist(err) {
		log.Errorf("%s is not initialized for packaging. Please run `hover init-packaging %s` first.", packagingFormat, packagingFormat)
		os.Exit(1)
	}
}

func removeDashesAndUnderscores(projectName string) string {
	return strings.ReplaceAll(strings.ReplaceAll(projectName, "-", ""), "_", "")
}

func printInitFinished(packagingFormat string) {
	log.Infof("go/packaging/%s has been created. You can modify the configuration files and add it to git.", packagingFormat)
	log.Infof(fmt.Sprintf("You now can package the %s using `%s`", strings.Split(packagingFormat, "-")[0], log.Au().Magenta("hover build "+packagingFormat)))
}

func printPackagingFinished(packagingFormat string) {
	log.Infof("Successfully packaged %s", strings.Split(packagingFormat, "-")[1])
}

func getTemporaryBuildDirectory(projectName string, packagingFormat string) string {
	tmpPath, err := ioutil.TempDir("", "hover-build-"+projectName+"-"+packagingFormat)
	if err != nil {
		log.Errorf("Couldn't get temporary build directory: %v", err)
		os.Exit(1)
	}
	return tmpPath
}

// AssertDockerInstalled check if docker is installed on the host os, otherwise exits with an error.
func AssertDockerInstalled() {
	if build.DockerBin == "" {
		log.Errorf("To use packaging, Docker needs to be installed.\nhttps://docs.docker.com/install")
		os.Exit(1)
	}
}

func getAuthor() string {
	author := pubspec.GetPubSpec().Author
	if author == "" {
		log.Warnf("Missing author field in pubspec.yaml")
		u, err := user.Current()
		if err != nil {
			log.Errorf("Couldn't get current user: %v", err)
			os.Exit(1)
		}
		author = u.Username
		log.Printf("Using this username from system instead: %s", author)
	}
	return author
}

func createDockerfile(packagingFormat string, dockerFileContent []string) {
	dockerFilePath, err := filepath.Abs(filepath.Join(packagingFormatPath(packagingFormat), "Dockerfile"))
	if err != nil {
		log.Errorf("Failed to resolve absolute path for Dockerfile %s: %v", dockerFilePath, err)
		os.Exit(1)
	}
	dockerFile, err := os.Create(dockerFilePath)
	if err != nil {
		log.Errorf("Failed to create Dockerfile %s: %v", dockerFilePath, err)
		os.Exit(1)
	}
	for _, line := range dockerFileContent {
		if _, err := dockerFile.WriteString(line + "\n"); err != nil {
			log.Errorf("Could not write Dockerfile: %v", err)
			os.Exit(1)
		}
	}
	err = dockerFile.Close()
	if err != nil {
		log.Errorf("Could not close Dockerfile: %v", err)
		os.Exit(1)
	}
}

func runDockerPackaging(path string, packagingFormat string, command []string) {
	dockerBuildCmd := exec.Command(build.DockerBin, "build", "-t", "hover-build-packaging-"+packagingFormat, ".")
	dockerBuildCmd.Stdout = os.Stdout
	dockerBuildCmd.Stderr = os.Stderr
	dockerBuildCmd.Dir = packagingFormatPath(packagingFormat)
	err := dockerBuildCmd.Run()
	if err != nil {
		log.Errorf("Docker build failed: %v", err)
		os.Exit(1)
	}
	u, err := user.Current()
	if err != nil {
		log.Errorf("Couldn't get current user: %v", err)
		os.Exit(1)
	}
	args := []string{
		"run",
		"-w", "/app",
		"-v", path + ":/app",
	}
	args = append(args, "hover-build-packaging-"+packagingFormat)
	chownStr := ""
	if runtime.GOOS != "windows" {
		chownStr = fmt.Sprintf(" && chown %s:%s * -R", u.Uid, u.Gid)
	}
	args = append(args, "bash", "-c", fmt.Sprintf("%s%s", strings.Join(command, " "), chownStr))
	dockerRunCmd := exec.Command(build.DockerBin, args...)
	dockerRunCmd.Stderr = os.Stderr
	dockerRunCmd.Stdout = os.Stdout
	dockerRunCmd.Dir = path
	err = dockerRunCmd.Run()
	if err != nil {
		log.Errorf("Docker run failed: %v", err)
		log.Warnf("Packaging is very experimental and has only been tested on Linux.")
		log.Infof("To help us debuging this error, please zip the content of:\n       \"%s\"\n       %s",
			log.Au().Blue(path),
			log.Au().Green("and try to package on another OS. You can also share this zip with the go-flutter team."))
		log.Infof("You can package the app without hover by running:")
		log.Infof("  `%s`", log.Au().Magenta("cd "+path))
		log.Infof("  docker build: `%s`", log.Au().Magenta(dockerBuildCmd.String()))
		log.Infof("  docker run: `%s`", log.Au().Magenta(dockerRunCmd.String()))
		os.Exit(1)
	}
}
