package handler

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

// EventHandler is an event handler for a multi-cluster Request.
type EventHandler = handler.TypedEventHandler[client.Object, mcreconcile.Request]
