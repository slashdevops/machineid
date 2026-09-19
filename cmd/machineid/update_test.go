package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestIsUpdateVerb(t *testing.T) {
	if !isUpdateVerb([]string{"update"}) || !isUpdateVerb([]string{"update", "-check"}) {
		t.Error("update verb not recognised")
	}
	if isUpdateVerb(nil) || isUpdateVerb([]string{"-cpu"}) || isUpdateVerb([]string{"-cpu", "update"}) {
		t.Error("false positive")
	}
}

func TestRunUpdateHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runUpdate([]string{"-h"}, &bytes.Buffer{}, &out, &errOut); code != exitUpdateOK {
		t.Errorf("exit = %d", code)
	}
	for _, want := range []string{"machineid update [options]", "-check", "-require-signature", "Exit Codes", "MACHINEID_UPDATE_BUDGET"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("help missing %q:\n%s", want, errOut.String())
		}
	}
}

func TestRunUpdateUsageErrors(t *testing.T) {
	tests := [][]string{
		{"-method", "bogus"},
		{"-nope"},
		{"extra"},
	}
	for _, args := range tests {
		var out, errOut bytes.Buffer
		if code := runUpdate(args, &bytes.Buffer{}, &out, &errOut); code != exitUpdateUsage {
			t.Errorf("runUpdate(%v) exit = %d, want %d\n%s", args, code, exitUpdateUsage, errOut.String())
		}
	}
}

func TestConfirmerNonInteractiveDeclines(t *testing.T) {
	var out, errOut bytes.Buffer
	ask := confirmer(strings.NewReader("y\n"), &out, &errOut)
	ok, err := ask("Update now?")
	if err != nil || ok {
		t.Errorf("a non-terminal stdin must decline: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(errOut.String(), "-yes") {
		t.Error(errOut.String())
	}
}
