package sync

import (
	"sort"
	"time"

	"github.com/launchrctl/launchr"
)

const (
	// SortAsc const.
	SortAsc = "asc"
	// SortDesc const.
	SortDesc = "desc"
)

// TimelineItem is interface for storing commit, date and version of propagated items.
// Storing such items in slice allows us to propagate items in the same order they were changed.
type TimelineItem interface {
	GetCommit() string
	GetVersion() string
	GetDate() time.Time
	Merge(item TimelineItem)
	Print()
}

// TimelineComponentsItem implements TimelineItem interface and stores Component map.
type TimelineComponentsItem struct {
	version    string
	commit     string
	components *OrderedMap[*Component]
	date       time.Time
	printer    *launchr.Terminal
}

// NewTimelineComponentsItem returns new instance of [TimelineComponentsItem]
func NewTimelineComponentsItem(version, commit string, date time.Time, printer *launchr.Terminal) *TimelineComponentsItem {
	return &TimelineComponentsItem{
		version:    version,
		commit:     commit,
		date:       date,
		components: NewOrderedMap[*Component](),
		printer:    printer,
	}
}

// GetCommit returns timeline item commit.
func (i *TimelineComponentsItem) GetCommit() string {
	return i.commit
}

// GetVersion returns timeline item version to propagate.
func (i *TimelineComponentsItem) GetVersion() string {
	return i.version
}

// GetDate returns timeline item date.
func (i *TimelineComponentsItem) GetDate() time.Time {
	return i.date
}

// AddComponent pushes [Component] into timeline item.
func (i *TimelineComponentsItem) AddComponent(c *Component) {
	i.components.Set(c.GetName(), c)
}

// GetComponents returns [Component] map of timeline.
func (i *TimelineComponentsItem) GetComponents() *OrderedMap[*Component] {
	return i.components
}

// Merge allows to merge other timeline item components.
func (i *TimelineComponentsItem) Merge(item TimelineItem) {
	if c2, ok := item.(*TimelineComponentsItem); ok {
		for _, key := range c2.components.Keys() {
			_, exists := i.components.Get(key)
			if exists {
				continue
			}

			itemComp, exists := c2.components.Get(key)
			if !exists {
				continue
			}

			i.components.Set(key, itemComp)
		}
	}
}

// Print outputs common item info.
func (i *TimelineComponentsItem) Print() {
	i.printer.Printfln("Version: %s, Date: %s, Commit: %s", i.GetVersion(), i.GetDate(), i.GetCommit())
	i.printer.Printf("Component List:\n")
	for _, key := range i.components.Keys() {
		v, ok := i.components.Get(key)
		if !ok {
			continue
		}
		i.printer.Printfln("- %s", v.GetName())
	}
}

// TimelineVariablesItem implements TimelineItem interface and stores Variable map.
type TimelineVariablesItem struct {
	version   string
	commit    string
	variables *OrderedMap[*Variable]
	date      time.Time
	printer   *launchr.Terminal
}

// NewTimelineVariablesItem returns new instance of [TimelineVariablesItem]
func NewTimelineVariablesItem(version, commit string, date time.Time, printer *launchr.Terminal) *TimelineVariablesItem {
	return &TimelineVariablesItem{
		version:   version,
		commit:    commit,
		date:      date,
		variables: NewOrderedMap[*Variable](),
		printer:   printer,
	}
}

// GetCommit returns timeline item commit.
func (i *TimelineVariablesItem) GetCommit() string {
	return i.commit
}

// GetVersion returns timeline item version to propagate.
func (i *TimelineVariablesItem) GetVersion() string {
	return i.version
}

// GetDate returns timeline item date.
func (i *TimelineVariablesItem) GetDate() time.Time {
	return i.date
}

// AddVariable pushes [Variable] into timeline item.
func (i *TimelineVariablesItem) AddVariable(v *Variable) {
	i.variables.Set(v.GetName(), v)
}

// GetVariables returns [Variable] map of timeline.
func (i *TimelineVariablesItem) GetVariables() *OrderedMap[*Variable] {
	return i.variables
}

// Merge allows to merge other timeline item variables.
func (i *TimelineVariablesItem) Merge(item TimelineItem) {
	if v2, ok := item.(*TimelineVariablesItem); ok {
		for _, key := range v2.variables.Keys() {
			_, exists := i.variables.Get(key)
			if exists {
				continue
			}

			itemVar, exists := v2.variables.Get(key)
			if !exists {
				continue
			}

			i.variables.Set(key, itemVar)
		}
	}
}

// Print outputs common item info.
func (i *TimelineVariablesItem) Print() {
	i.printer.Printfln("Version: %s, Date: %s, Commit: %s", i.GetVersion(), i.GetDate(), i.GetCommit())
	i.printer.Printf("Variable List:\n")
	for _, key := range i.variables.Keys() {
		v, ok := i.variables.Get(key)
		if !ok {
			continue
		}
		i.printer.Printfln("- %s", v.GetName())
	}
}

// AddToTimeline inserts items into timeline slice.
func AddToTimeline(list []TimelineItem, item TimelineItem) []TimelineItem {
	for _, i := range list {
		switch i.(type) {
		case *TimelineVariablesItem:
			if _, ok := item.(*TimelineVariablesItem); !ok {
				continue
			}
		case *TimelineComponentsItem:
			if _, ok := item.(*TimelineComponentsItem); !ok {
				continue
			}
		default:
			continue
		}

		if i.GetVersion() == item.GetVersion() && i.GetDate().Equal(item.GetDate()) {
			i.Merge(item)
			return list
		}
	}

	return append(list, item)
}

// SortTimeline sorts timeline items in slice.
func SortTimeline(list []TimelineItem, order string) {
	sort.Slice(list, func(i, j int) bool {
		dateI := list[i].GetDate()
		dateJ := list[j].GetDate()

		// Determine the date comparison based on the order
		if !dateI.Equal(dateJ) {
			if order == SortAsc {
				return dateI.Before(dateJ)
			}
			return dateI.After(dateJ)
		}

		// If dates are the same, prioritize by type
		switch list[i].(type) {
		case *TimelineVariablesItem:
			switch list[j].(type) {
			case *TimelineVariablesItem:
				// Both are Variables, maintain current order
				return false
			case *TimelineComponentsItem:
				// Variables come before Components if asc, after if desc
				return order == SortAsc
			default:
				// Variables come before unknown types
				return true
			}
		case *TimelineComponentsItem:
			switch list[j].(type) {
			case *TimelineVariablesItem:
				// Components come after Variables if asc, before if desc
				return order == SortDesc
			case *TimelineComponentsItem:
				// Both are Components, maintain current order
				return false
			default:
				// Components come before unknown types
				return true
			}
		default:
			switch list[j].(type) {
			case *TimelineVariablesItem, *TimelineComponentsItem:
				// Unknown types come after Variables and Components
				return false
			default:
				// Maintain current order for unknown types
				return false
			}
		}
	})
}

// CreateTimeline returns fresh timeline slice.
func CreateTimeline() []TimelineItem {
	return make([]TimelineItem, 0)
}
