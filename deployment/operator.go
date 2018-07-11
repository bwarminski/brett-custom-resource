package main

import "github.com/golang/glog"

func (c *Controller) handle(key string) error {
	glog.Infof("Handling '%s'")
	return nil
}