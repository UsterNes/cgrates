/*
Real-time Online/Offline Charging System (OerS) for Telecom & ISP environments
Copyright (C) ITsysCOM GmbH

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>
*/

package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cgrates/cgrates/config"
	"github.com/cgrates/cgrates/engine"
	"github.com/cgrates/cgrates/utils"
	"github.com/cgrates/ltcache"
)

// actionTarget returns the target attached to an action
func actionTarget(act string) (trgt string) {
	switch act {
	default:
		trgt = utils.MetaNone
	}
	return
}

func newScheduledActs(tenant, apID, trgTyp, trgID, schedule string,
	ctx context.Context, data utils.MapStorage, acts []actioner) (sActs *scheduledActs) {
	return &scheduledActs{tenant, apID, trgTyp, trgID, schedule, ctx, data, acts,
		ltcache.NewTransCache(map[string]*ltcache.CacheConfig{})}
}

// scheduled is a set of actions which will be executed directly or by the cron.schedule
type scheduledActs struct {
	tenant, apID, trgTyp, trgID string
	schedule                    string
	ctx                         context.Context
	data                        utils.MapStorage
	acts                        []actioner

	cch *ltcache.TransCache // cache data between actions here
}

// Execute is called when we want the ActionProfile to be executed
func (s *scheduledActs) ScheduledExecute() {
	s.Execute()
}

// Execute notifies possible errors on execution
func (s *scheduledActs) Execute() (err error) {
	var partExec bool
	for _, act := range s.acts {
		//ctx, cancel := context.WithTimeout(s.ctx, act.cfg().TTL)
		if err := act.execute(s.ctx, s.data); err != nil {
			utils.Logger.Warning(fmt.Sprintf("executing action: <%s>, error: <%s>", act.id(), err))
			partExec = true
		}
	}
	// postexec here
	if partExec {
		err = utils.ErrPartiallyExecuted
	}
	return
}

// postExec will save data which was modified in actions and unlock guardian
func (s *scheduledActs) postExec() (err error) {
	return
}

// newActionersFromActions constructs multiple actioners out of APAction configurations
func newActionersFromActions(cfg *config.CGRConfig, fltrS *engine.FilterS, dm *engine.DataManager,
	connMgr *engine.ConnManager, aCfgs []*engine.APAction, tnt string) (acts []actioner, err error) {
	acts = make([]actioner, len(aCfgs))
	for i, aCfg := range aCfgs {
		if acts[i], err = newActioner(cfg, fltrS, dm, connMgr, aCfg, tnt); err != nil {
			return nil, err
		}
	}
	return
}

// newAction is the constructor to create actioner
func newActioner(cfg *config.CGRConfig, fltrS *engine.FilterS, dm *engine.DataManager,
	connMgr *engine.ConnManager, aCfg *engine.APAction, tnt string) (act actioner, err error) {
	switch aCfg.Type {
	case utils.MetaLog:
		return &actLog{aCfg}, nil
	case utils.CDRLog:
		return &actCDRLog{config: cfg, connMgr: connMgr, aCfg: aCfg, filterS: fltrS}, nil
	case utils.MetaHTTPPost:
		return &actHTTPPost{aCfg: aCfg}, nil
	case utils.MetaExport:
		return &actExport{config: cfg, connMgr: connMgr, aCfg: aCfg, tnt: tnt}, nil
	case utils.MetaResetStatQueue:
		return &actResetStat{config: cfg, connMgr: connMgr, aCfg: aCfg, tnt: tnt}, nil
	case utils.MetaResetThreshold:
		return &actResetThreshold{config: cfg, connMgr: connMgr, aCfg: aCfg, tnt: tnt}, nil
	default:
		return nil, fmt.Errorf("unsupported action type: <%s>", aCfg.Type)

	}
}

// actioner is implemented by each action type
type actioner interface {
	id() string
	cfg() *engine.APAction
	execute(ctx context.Context, data utils.MapStorage) (err error)
}

// actLogger will log data to CGRateS logger
type actLog struct {
	aCfg *engine.APAction
}

func (aL *actLog) id() string {
	return aL.aCfg.ID
}

func (aL *actLog) cfg() *engine.APAction {
	return aL.aCfg
}

// execute implements actioner interface
func (aL *actLog) execute(ctx context.Context, data utils.MapStorage) (err error) {
	var body []byte
	if body, err = json.Marshal(data); err != nil {
		return
	}
	utils.Logger.Info(fmt.Sprintf("LOG Event: %s", body))
	return
}

// actCDRLog will log data to CGRateS logger
type actCDRLog struct {
	config  *config.CGRConfig
	filterS *engine.FilterS
	connMgr *engine.ConnManager
	aCfg    *engine.APAction
}

func (aL *actCDRLog) id() string {
	return aL.aCfg.ID
}

func (aL *actCDRLog) cfg() *engine.APAction {
	return aL.aCfg
}

// execute implements actioner interface
func (aL *actCDRLog) execute(ctx context.Context, data utils.MapStorage) (err error) {
	if len(aL.config.ActionSCfg().CDRsConns) == 0 {
		//eroare predefinita
		return fmt.Errorf("no connection with CDR Server")
	}
	template := aL.config.TemplatesCfg()[utils.MetaCdrLog]
	if id, has := aL.cfg().Opts[utils.MetaTemplateID]; has { // if templateID is not present we use default template
		template = aL.config.TemplatesCfg()[utils.IfaceAsString(id)]
	}
	// split the data into Request and Opts to send as parameters to AgentRequest
	req := data[utils.MetaReq].(map[string]interface{})
	reqNm := utils.MapStorage{}
	for key, val := range req {
		reqNm[key] = val
	}

	opts := data[utils.MetaOpts].(map[string]interface{})
	optsMS := utils.MapStorage{}
	for key, val := range opts {
		optsMS[key] = val
	}
	optsNm := utils.NewOrderedNavigableMap()
	for key, val := range opts {
		optsNm.Set(utils.NewFullPath(key, utils.NestingSep), utils.NewNMData(val))
	}

	oNm := map[string]*utils.OrderedNavigableMap{
		utils.MetaCDR:  utils.NewOrderedNavigableMap(),
		utils.MetaOpts: optsNm,
	}
	// construct an AgentRequest so we can build the reply and send it to CDRServer
	cdrLogReq := engine.NewEventRequest(reqNm, nil, optsMS, nil, aL.config.GeneralCfg().DefaultTenant,
		aL.config.GeneralCfg().DefaultTimezone, aL.filterS, oNm)

	if err = cdrLogReq.SetFields(template); err != nil {
		return
	}
	var rply string
	if err := aL.connMgr.Call(aL.config.ActionSCfg().CDRsConns, nil,
		utils.CDRsV1ProcessEvent,
		&engine.ArgV1ProcessEvent{
			Flags:    []string{utils.ConcatenatedKey(utils.MetaChargers, "false")}, // do not try to get the chargers for cdrlog
			CGREvent: *config.NMAsCGREvent(cdrLogReq.OrdNavMP[utils.MetaCDR], cdrLogReq.Tenant, utils.NestingSep, cdrLogReq.OrdNavMP[utils.MetaOpts]),
		}, &rply); err != nil {
		return err
	}

	return
}

type actHTTPPost struct {
	aCfg *engine.APAction
}

func (aL *actHTTPPost) id() string {
	return aL.aCfg.ID
}

func (aL *actHTTPPost) cfg() *engine.APAction {
	return aL.aCfg
}

// execute implements actioner interface
func (aL *actHTTPPost) execute(ctx context.Context, data utils.MapStorage) (err error) {
	var body []byte
	if body, err = json.Marshal(data); err != nil {
		return
	}
	var pstr *engine.HTTPPoster
	if pstr, err = engine.NewHTTPPoster(config.CgrConfig().GeneralCfg().ReplyTimeout, aL.cfg().Path,
		utils.ContentJSON, config.CgrConfig().GeneralCfg().PosterAttempts); err != nil {
		return
	}
	if async, has := aL.cfg().Opts[utils.MetaAsync]; has && utils.IfaceAsString(async) == utils.TrueStr {
		go func() {
			err := pstr.PostValues(body, make(http.Header))
			if err != nil && config.CgrConfig().GeneralCfg().FailedPostsDir != utils.MetaNone {
				engine.AddFailedPost(aL.cfg().Path, utils.MetaHTTPjson, utils.ActionsPoster+utils.HierarchySep+aL.cfg().Type, body, make(map[string]interface{}))
			}
		}()
		return
	}
	if err = pstr.PostValues(body, make(http.Header)); err != nil && config.CgrConfig().GeneralCfg().FailedPostsDir != utils.MetaNone {
		engine.AddFailedPost(aL.cfg().Path, utils.MetaHTTPjson, utils.ActionsPoster+utils.HierarchySep+aL.cfg().Type, body, make(map[string]interface{}))
		err = nil
	}
	return
}

type actExport struct {
	tnt     string
	config  *config.CGRConfig
	connMgr *engine.ConnManager
	aCfg    *engine.APAction
}

func (aL *actExport) id() string {
	return aL.aCfg.ID
}

func (aL *actExport) cfg() *engine.APAction {
	return aL.aCfg
}

// execute implements actioner interface
func (aL *actExport) execute(ctx context.Context, data utils.MapStorage) (err error) {
	var exporterIDs []string
	if expIDs, has := aL.cfg().Opts[utils.MetaExporterIDs]; has { // if templateID is not present we use default template
		exporterIDs = strings.Split(utils.IfaceAsString(expIDs), utils.InfieldSep)
	}
	args := &utils.CGREventWithEeIDs{
		EeIDs: exporterIDs,
		CGREvent: &utils.CGREvent{
			Tenant: aL.tnt,
			Time:   utils.TimePointer(time.Now()),
			ID:     utils.GenUUID(),
			Event:  data[utils.MetaReq].(map[string]interface{}),
			Opts:   data[utils.MetaOpts].(map[string]interface{}),
		},
	}

	var rply map[string]map[string]interface{}
	return aL.connMgr.Call(aL.config.ActionSCfg().EEsConns, nil,
		utils.EeSv1ProcessEvent, args, &rply)
}

type actResetStat struct {
	tnt     string
	config  *config.CGRConfig
	connMgr *engine.ConnManager
	aCfg    *engine.APAction
}

func (aL *actResetStat) id() string {
	return aL.aCfg.ID
}

func (aL *actResetStat) cfg() *engine.APAction {
	return aL.aCfg
}

// execute implements actioner interface
func (aL *actResetStat) execute(ctx context.Context, data utils.MapStorage) (err error) {
	var tenID string
	if tenID, err = aL.cfg().Value.ParseDataProvider(data); err != nil {
		return
	}
	args := &utils.TenantIDWithOpts{
		TenantID: utils.NewTenantID(tenID),
		Opts:     data[utils.MetaOpts].(map[string]interface{}),
	}
	if args.Tenant == utils.EmptyString { // in case that user pass only ID we populate the tenant from the event
		args.Tenant = aL.tnt
	}
	var rply string
	return aL.connMgr.Call(aL.config.ActionSCfg().StatSConns, nil,
		utils.StatSv1ResetStatQueue, args, &rply)
}

type actResetThreshold struct {
	tnt     string
	config  *config.CGRConfig
	connMgr *engine.ConnManager
	aCfg    *engine.APAction
}

func (aL *actResetThreshold) id() string {
	return aL.aCfg.ID
}

func (aL *actResetThreshold) cfg() *engine.APAction {
	return aL.aCfg
}

// execute implements actioner interface
func (aL *actResetThreshold) execute(ctx context.Context, data utils.MapStorage) (err error) {
	var tenID string
	if tenID, err = aL.cfg().Value.ParseDataProvider(data); err != nil {
		return
	}
	args := &utils.TenantIDWithOpts{
		TenantID: utils.NewTenantID(tenID),
		Opts:     data[utils.MetaOpts].(map[string]interface{}),
	}
	if args.Tenant == utils.EmptyString { // in case that user pass only ID we populate the tenant from the event
		args.Tenant = aL.tnt
	}
	var rply string
	return aL.connMgr.Call(aL.config.ActionSCfg().ThresholdSConns, nil,
		utils.ThresholdSv1ResetThreshold, args, &rply)
}
