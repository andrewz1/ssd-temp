package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	maxDiff  = 5
	checkInt = maxDiff * time.Second
	sysPath  = "/sys/class/hwmon/hwmon*"
	namePath = "/name"

	gpuName      = "amdgpu"
	gpuFanMode   = "/pwm1_enable"
	gpuFanPwm    = "/pwm1"
	gpuFanPwmMin = "/pwm1_min"
	gpuFanPwmMax = "/pwm1_max"
	gpuFanSpeed  = "/fan1_input"

	ssdName    = "nvme"
	ssdTemp    = "/temp1_input"
	ssdTempMin = 32000
	ssdTempMax = 55000
)

var (
	gpuPath string
	ssdPath string
	pwmMin  int
	pwmMax  int
	pwmLast int
)

func readInt(path string) (rv int, err error) {
	var f *os.File
	if f, err = os.Open(path); err != nil {
		return
	}
	defer f.Close()
	var t, n int
	if n, err = fmt.Fscanf(f, "%d", &t); err != nil {
		return
	}
	if n != 1 {
		err = errors.New("invalid file content")
		return
	}
	rv = t
	return
}

func writeInt(path string, v int) (err error) {
	var f *os.File
	if f, err = os.OpenFile(path, os.O_WRONLY, 0644); err != nil {
		return
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%d", v)
	return
}

func getFanMode() (mode int, err error) {
	path := gpuPath + gpuFanMode
	mode, err = readInt(path)
	return
}

func setFanMode(mode int) (err error) {
	path := gpuPath + gpuFanMode
	err = writeInt(path, mode)
	return
}

func getPwmRange() (err error) {
	path := gpuPath + gpuFanPwmMin
	if pwmMin, err = readInt(path); err != nil {
		return
	}
	path = gpuPath + gpuFanPwmMax
	if pwmMax, err = readInt(path); err != nil {
		return
	}
	return
}

func setFanPwm(pwm int) (err error) {
	path := gpuPath + gpuFanPwm
	err = writeInt(path, pwm)
	return
}

func getSsdTemp() (rv int, err error) {
	path := gpuPath + ssdTemp
	if rv, err = readInt(path); err != nil {
		return
	}
	return
}

func calcFanPwm() (rv int) {
	var (
		err  error
		temp int
	)
	if temp, err = getSsdTemp(); err != nil {
		rv = pwmMax
		return
	}
	switch {
	case temp <= ssdTempMin:
		rv = pwmMin
		return
	case temp >= ssdTempMax:
		rv = pwmMax
		return
	}
	tPrc := float64(temp-ssdTempMin) / float64(ssdTempMax-ssdTempMin)
	pwm := float64(pwmMax-pwmMin) * tPrc
	rv = pwmMin + int(pwm)
	return
}

func calcFanPwmWithDiff() (rv int) {
	calc := calcFanPwm()
	switch {
	case calc == pwmLast:
		rv = calc
	case calc > pwmLast:
		diff := calc - pwmLast
		if diff > maxDiff {
			rv = pwmLast + maxDiff
		} else {
			rv = calc
		}
	case calc < pwmLast:
		diff := pwmLast - calc
		if diff > maxDiff {
			rv = pwmLast - maxDiff
		} else {
			rv = calc
		}
	}
	pwmLast = rv
	return
}

func setFanModePwm(pwm int) (err error) {
	if err = setFanMode(1); err != nil {
		fmt.Fprintln(os.Stderr, "can't set FAN mode:", err)
		return
	}
	if err = setFanPwm(pwm); err != nil {
		fmt.Fprintln(os.Stderr, "can't set FAN PWM:", err)
		return
	}
	return
}

func main() {
	var (
		err      error
		nameData []byte
		gpuSet   bool
		ssdSet   bool
	)
	paths, _ := filepath.Glob(sysPath)
	for _, p := range paths {
		if nameData, err = ioutil.ReadFile(p + namePath); err != nil {
			continue
		}
		if gpuSet && ssdSet {
			break
		}
		if !gpuSet && bytes.HasPrefix(nameData, []byte(gpuName)) {
			gpuPath = p
			gpuSet = true
		}
		if !ssdSet && bytes.HasPrefix(nameData, []byte(ssdName)) {
			ssdPath = p
			ssdSet = true
		}
	}
	if !gpuSet {
		fmt.Fprintln(os.Stderr, "can't detect GPU")
		os.Exit(1)
	}
	if !ssdSet {
		fmt.Fprintln(os.Stderr, "can't detect SSD")
		os.Exit(1)
	}
	fmt.Println("GPUpath:", gpuPath)
	fmt.Println("SSDpath:", ssdPath)
	if err = getPwmRange(); err != nil {
		fmt.Fprintln(os.Stderr, "can't detect PWM range", err)
		os.Exit(1)
	}
	var modeOld int
	if modeOld, err = getFanMode(); err != nil {
		fmt.Fprintln(os.Stderr, "can't get FAN mode:", err)
		os.Exit(1)
	}
	pwmLast = calcFanPwm()
	if err = setFanModePwm(pwmLast); err != nil {
		os.Exit(1)
	}
	sc := make(chan os.Signal, 1)
	signal.Notify(sc,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGINT,
		syscall.SIGHUP,
	)
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go worker(done, &wg)
	fmt.Println("exit:", <-sc)
	close(done)
	wg.Wait()
	setFanMode(modeOld)
}

func worker(done chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	ticker := time.NewTicker(checkInt)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			setFanModePwm(calcFanPwmWithDiff())
		}
	}
}
