package main

import (
	"context"
	"devtest/model"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var timeTemplate = "2006-01-02T15-04-05"

var (
	workerPools = make(chan struct{}, runtime.NumCPU())
	wg          = &sync.WaitGroup{}
	mu          = &sync.Mutex{}
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	endChan := make(chan os.Signal, 1)
	signal.Notify(endChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		select {
		case <-endChan:
			log.Println("context cancelled, terminating application...")
			cancel()
			os.Exit(1)
		case <-ctx.Done():
			return
		}
	}()

	log.Println("total worker:", cap(workerPools))

	// directory to video files
	var result model.Output
	f, err := os.Open("./record") // change to the directory of the video files
	if err != nil {
		fmt.Println(err)
		return
	}

	files, err := f.Readdir(0)
	if err != nil {
		fmt.Println(err)
		return
	}

	// sort files by name ascending
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	//uncomment one set of start time and end time for testing function purpose
	startTime := time.Date(2024, time.January, 9, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, time.January, 10, 0, 0, 0, 0, time.UTC)
	//startTime := time.Date(2024, time.January, 10, 0, 0, 0, 0, time.UTC)
	//endTime := time.Date(2024, time.January, 11, 0, 0, 0, 0, time.UTC)
	//startTime := time.Date(2024, time.January, 11, 0, 0, 0, 0, time.UTC)
	//endTime := time.Date(2024, time.January, 12, 0, 0, 0, 0, time.UTC)

	result.Result.TotalTime = int(endTime.Sub(startTime).Seconds())
	result.Result.ErrorTime = 0
	result.Result.RecordTime = 0

	result.Result.TotalRecording = 0
	result.StartTime = &startTime
	result.EndTime = &endTime
	var errorFile []model.Error
	var recordingFile []model.RecordingFile

	wg.Add(len(files))

	for i, v := range files {
		select {
		case workerPools <- struct{}{}:
			go func(i int, v fs.FileInfo) {
				videoFilePath := "./record/" + v.Name() //get directory for mmpeg
				startTime := getTime(v.Name())

				//only proceeds file within range
				if startTime.Before(endTime.Add(1*time.Millisecond)) && startTime.After(startTime.Add(-1*time.Millisecond)) {
					numDuration, err := getDuration(ctx, videoFilePath)
					if err != nil {
						return
					}

					mu.Lock()
					result.Result.TotalRecording++          //mutex
					result.Result.RecordTime += numDuration //mutex
					mu.Unlock()

					var recordingInfo model.RecordingFile
					recordingInfo.Filename = v.Name()
					recordingInfo.Duration = numDuration
					recordingFile = append(recordingFile, recordingInfo)
					// handle if vid count as error
					var rangeToNextFile int //duration to the next file
					//check if the last video is exactly 5 minutes and there's a video gap
					if i < len(files)-1 {
						nextStartTime := getTime(files[i+1].Name())
						if nextStartTime.Before(endTime.Add(1*time.Millisecond)) && nextStartTime.After(nextStartTime.Add(-1*time.Millisecond)) {
							rangeToNextFile = int(nextStartTime.Sub(startTime).Seconds())
						} else {
							rangeToNextFile = 0
						}
					} else {
						rangeToNextFile = 0
					}
					if numDuration != 300 || (rangeToNextFile > 300) { //if duration isn't 5 minutes then count as error
						endTime := startTime.Add(time.Duration(numDuration) * time.Second) //end time of the error video
						nextVidStartTime := getTime(files[i+1].Name())                     //get the start time of the next video
						errorDuration := nextVidStartTime.Sub(endTime)                     //get the time between the end time of the error video and the start time of the next video
						var errorInfo model.Error
						errorInfo.Filename = v.Name()
						errorInfo.Duration = int(errorDuration.Seconds())
						errorInfo.TimeError.StartTime = &endTime
						errorInfo.TimeError.EndTime = &nextVidStartTime
						// add to result
						errorFile = append(errorFile, errorInfo)

						mu.Lock()
						result.Result.TotalError++
						result.Result.ErrorTime += errorInfo.Duration
						mu.Unlock()
					}
				}
				<-workerPools
				wg.Done()
			}(i, v)
		case <-ctx.Done():
			return
		}
	}

	wg.Wait()

	//sort File since goroutine results might be random
	sort.Slice(errorFile, func(i, j int) bool {
		return errorFile[i].TimeError.StartTime.Before(*errorFile[j].TimeError.StartTime)
	})
	//sort recording file
	sort.Slice(recordingFile, func(i, j int) bool {
		return recordingFile[i].Filename < recordingFile[j].Filename
	})
	result.Result.RecordingFile = recordingFile
	result.Result.Error = errorFile
	result.Result.ErrorPercentage = float64(result.Result.ErrorTime) / float64(result.Result.TotalTime) * 100
	result.Result.RecordPercentage = float64(result.Result.RecordTime) / float64(result.Result.TotalTime) * 100
	jsonOutput, err := json.Marshal(result)
	if err != nil {
		fmt.Println("Error:", err)
		return

	}
	fmt.Println(string(jsonOutput))
	fmt.Println(result.Result.TotalRecording)
}

// getTime from filename
func getTime(s string) time.Time {
	timeString := strings.TrimSuffix(s, ".mp4")
	t, err := time.Parse(timeTemplate, timeString)
	if err != nil {
		fmt.Println(err)
		panic(err)

	}
	return t
}
func getDuration(ctx context.Context, videoFilePath string) (int, error) {
	//download ffmpeg first and add to cmd 	environment variables
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", videoFilePath)
	output, err := cmd.CombinedOutput() // output still in ascii
	if err != nil {
		fmt.Println("Error:", err)
		return 0, err
	}

	splitOutput := strings.Split(string(output), ".")
	numDuration, err := strconv.Atoi(string(splitOutput[0]))
	if err != nil {
		fmt.Println("Error:", err)
		return 0, err
	}

	return numDuration, nil
}
