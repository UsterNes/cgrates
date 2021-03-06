/*
Real-time Online/Offline Charging System (OCS) for Telecom & ISP environments
Copyright (C) ITsysCOM GmbH

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT MetaAny WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>
*/

package engine

import (
	"github.com/cgrates/cgrates/utils"
)

func NewWeightSorter(rS *RouteService) *WeightSorter {
	return &WeightSorter{rS: rS,
		sorting: utils.MetaWeight}
}

// WeightSorter orders routes based on their weight, no cost involved
type WeightSorter struct {
	sorting string
	rS      *RouteService
}

func (ws *WeightSorter) SortRoutes(prflID string,
	routes map[string]*Route, suplEv *utils.CGREvent, extraOpts *optsGetRoutes) (sortedRoutes *SortedRoutes, err error) {
	sortedRoutes = &SortedRoutes{ProfileID: prflID,
		Sorting:      ws.sorting,
		SortedRoutes: make([]*SortedRoute, 0)}
	for _, route := range routes {
		if srtRoute, pass, err := ws.rS.populateSortingData(suplEv, route, extraOpts); err != nil {
			return nil, err
		} else if pass && srtRoute != nil {
			sortedRoutes.SortedRoutes = append(sortedRoutes.SortedRoutes, srtRoute)
		}
	}
	sortedRoutes.SortWeight()
	return
}
