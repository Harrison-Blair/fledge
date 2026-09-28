// Package message implements agent message: acknowledged prompt submission.
// message.go validates options, delivers to one target, and renders results;
// fanout.go delivers the same message to several targets in order and renders one line per target.
package message
