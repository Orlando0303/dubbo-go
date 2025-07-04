/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package router

import (
	"github.com/RoaringBitmap/roaring"
)

import (
	"dubbo.apache.org/dubbo-go/v3/common"
	"dubbo.apache.org/dubbo-go/v3/protocol/base"
)

// PriorityRouterFactory creates priority router with url
type PriorityRouterFactory interface {
	// NewPriorityRouter creates router instance with URL
	NewPriorityRouter() (PriorityRouter, error)
}

// PriorityRouter routes with priority
type PriorityRouter interface {
	// Route Determine the target invokers list.
	Route([]base.Invoker, *common.URL, base.Invocation) []base.Invoker

	// URL Return URL in router
	URL() *common.URL

	// Priority Return Priority in router
	// 0 to ^int(0) is better
	Priority() int64

	// Notify the router the invoker list
	Notify(invokers []base.Invoker)
}

// Poolable caches address pool and address metadata for a router instance which will be used later in Router's Route.
type Poolable interface {
	// Pool created address pool and address metadata from the invokers.
	Pool([]base.Invoker) (AddrPool, AddrMetadata)

	// ShouldPool returns if it should pool. One typical scenario is a router rule changes, in this case, a pooling
	// is necessary, even if the addresses not changed at all.
	ShouldPool() bool

	// Name return the Poolable's name.
	Name() string
}

// AddrPool is an address pool, backed by a snapshot of address list, divided into categories.
type AddrPool map[string]*roaring.Bitmap

// AddrMetadata is address metadata, collected from a snapshot of address list by a router, if it implements Poolable.
type AddrMetadata interface {
	// Source indicates where the metadata comes from.
	Source() string
}

// Cache caches all addresses relevant info for a snapshot of received invokers. It keeps a snapshot of the received
// address list, and also keeps address pools and address metadata from routers based on the same address snapshot, if
// the router implements Poolable.
type Cache interface {
	// GetInvokers returns the snapshot of received invokers.
	GetInvokers() []base.Invoker

	// FindAddrPool returns address pool associated with the given Poolable instance.
	FindAddrPool(Poolable) AddrPool

	// FindAddrMeta returns address metadata associated with the given Poolable instance.
	FindAddrMeta(Poolable) AddrMetadata
}
