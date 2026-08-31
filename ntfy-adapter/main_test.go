package main

import "testing"

func TestBuildNotificationFiringCritical(t *testing.T) {
	a := alert{
		Status: "firing",
		Labels: map[string]string{
			"alertname":     "NodeDown",
			"severity":      "critical",
			"instance_name": "my-laptop",
		},
		Annotations: map[string]string{
			"summary": "node target my-laptop is down",
		},
	}

	n := buildNotification(a)

	if n.title != "NodeDown" {
		t.Errorf("got title %q, want %q", n.title, "NodeDown")
	}
	if n.priority != "urgent" {
		t.Errorf("got priority %q, want %q", n.priority, "urgent")
	}
	if n.tags != "rotating_light" {
		t.Errorf("got tags %q, want %q", n.tags, "rotating_light")
	}
	if n.message != "node target my-laptop is down" {
		t.Errorf("got message %q, want the summary annotation", n.message)
	}
}

func TestBuildNotificationFiringNonCritical(t *testing.T) {
	a := alert{
		Status: "firing",
		Labels: map[string]string{
			"alertname":     "HighCPU",
			"severity":      "warning",
			"instance_name": "web1",
		},
		Annotations: map[string]string{
			"summary": "web1 CPU usage is high",
		},
	}

	n := buildNotification(a)

	if n.priority != "default" {
		t.Errorf("got priority %q, want %q (non-critical must not be urgent)", n.priority, "default")
	}
	if n.tags != "warning" {
		t.Errorf("got tags %q, want %q", n.tags, "warning")
	}
}

func TestBuildNotificationFiringWithoutSummaryFallsBack(t *testing.T) {
	a := alert{
		Status: "firing",
		Labels: map[string]string{
			"alertname":     "NodeDown",
			"severity":      "critical",
			"instance_name": "web1",
		},
		Annotations: map[string]string{},
	}

	n := buildNotification(a)

	want := "web1 is firing"
	if n.message != want {
		t.Errorf("got message %q, want %q", n.message, want)
	}
}

func TestBuildNotificationResolved(t *testing.T) {
	a := alert{
		Status: "resolved",
		Labels: map[string]string{
			"alertname":     "NodeDown",
			"severity":      "critical",
			"instance_name": "my-laptop",
		},
		Annotations: map[string]string{
			"summary": "node target my-laptop is down",
		},
	}

	n := buildNotification(a)

	if n.title != "NodeDown resolved" {
		t.Errorf("got title %q, want %q", n.title, "NodeDown resolved")
	}
	if n.priority != "default" {
		t.Errorf("got priority %q, want %q", n.priority, "default")
	}
	if n.tags != "white_check_mark" {
		t.Errorf("got tags %q, want %q", n.tags, "white_check_mark")
	}
	want := "my-laptop has recovered"
	if n.message != want {
		t.Errorf("got message %q, want %q", n.message, want)
	}
}

func TestBuildNotificationResolvedIgnoresSeverity(t *testing.T) {
	a := alert{
		Status: "resolved",
		Labels: map[string]string{
			"alertname":     "NodeDown",
			"severity":      "critical",
			"instance_name": "web1",
		},
	}

	n := buildNotification(a)

	if n.priority == "urgent" {
		t.Error("resolved notification must not be urgent regardless of severity")
	}
}
