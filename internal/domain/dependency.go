package domain

import "time"

type Dependency struct {
	ID		  int64
	DependsOn int64
	CreatedAt time.Time
}