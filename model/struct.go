package model

import "time"

type Output struct {
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
	Result    Result     `json:"result"`
}

type Result struct {
	ErrorPercentage  float64         `json:"error_percentage"`
	ErrorTime        int             `json:"error_time"` //sekon
	RecordPercentage float64         `json:"record_percentage"`
	RecordTime       int             `json:"record_time"` //sekon
	TotalError       int             `json:"total_error"`
	TotalRecording   int             `json:"total_recording"`
	TotalTime        int             `json:"total_time"` //sekon
	Error            []Error         `json:"error"`
	RecordingFile    []RecordingFile `json:"recording_file"`
}

type Error struct {
	Duration  int       `json:"duration"` //sekon
	Filename  string    `json:"filename"`
	TimeError TimeError `json:"time_error"`
}

type TimeError struct {
	EndTime   *time.Time `json:"end_time"`
	StartTime *time.Time `json:"start_time"`
}
type RecordingFile struct {
	Duration int    `json:"duration"` //sekon
	Filename string `json:"filename"`
}
