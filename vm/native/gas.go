package native

import (
	"encoding/json"
	"fmt"

	"github.com/iost-official/go-iost/ilog"
	"github.com/iost-official/go-iost/vm/database"

	"github.com/iost-official/go-iost/common"
	"github.com/iost-official/go-iost/core/contract"
	"github.com/iost-official/go-iost/vm/host"
)

// UnpledgeFreezeSeconds coins will be frozen for 3 days after being unpledged
const UnpledgeFreezeSeconds int64 = 3 * 24 * 3600

var gasABIs *abiSet

// GasContractName the contract name
const GasContractName = "gas.iost"

func init() {
	gasABIs = newAbiSet()
	gasABIs.Register(initFunc, true)
	gasABIs.Register(constructor)
	gasABIs.Register(pledgeGas)
	gasABIs.Register(unpledgeGas)
	gasABIs.Register(transferTGas)
}

// Pledge Change all gas related storage here. If pledgeAmount > 0. pledge. If pledgeAmount < 0, unpledge.
func pledge(h *host.Host, pledger string, name string, pledgeAmountF *common.Fixed) (contract.Cost, error) {
	finalCost := contract.Cost0()
	if pledgeAmountF.IsZero() {
		return finalCost, fmt.Errorf("invalid pledge amount %v", pledgeAmountF.ToString())
	}
	if pledgeAmountF.IsNegative() {
		// do some checking
		unpledgeAmount := pledgeAmountF.Neg()
		// check total pledge is valid
		oldTotalPledge, cost := h.GasPledgeTotal(name)
		finalCost.AddAssign(cost)
		if oldTotalPledge.Sub(unpledgeAmount).LessThan(database.GasMinPledgeOfUser) {
			return finalCost, fmt.Errorf("unpledge to much %v - %v less than %v", oldTotalPledge.ToString(), unpledgeAmount.ToString(), database.GasMinPledgeOfUser.ToString())
		}
		// check personal pledge
		pledged, cost := h.GasManager.GasPledge(name, pledger)
		finalCost.AddAssign(cost)
		newPledge := pledged.Sub(unpledgeAmount)
		if newPledge.IsNegative() {
			return finalCost, fmt.Errorf("you cannot unpledge more than your pledge %v > %v", unpledgeAmount, pledged)
		}
	}

	limitDelta := pledgeAmountF.Multiply(database.GasLimit)
	totalDelta := pledgeAmountF
	gasDelta := pledgeAmountF.Multiply(database.GasImmediateReward)
	if pledgeAmountF.IsNegative() {
		// unpledge should not change current generated gas
		gasDelta = database.EmptyGas()
	}
	//fmt.Printf("limitd rated gasd %v %v %v\n", limitDelta, totalDelta, gasDelta)

	// pledge first time
	t, cost := h.GasManager.GasUpdateTime(name)
	finalCost.AddAssign(cost)
	if t == 0 {
		if pledgeAmountF.IsNegative() {
			return finalCost, fmt.Errorf("cannot unpledge! No pledge before")
		}
		cost = h.GasManager.SetGasPledge(name, pledger, pledgeAmountF)
		finalCost.AddAssign(cost)
		cost = h.GasManager.SetGasUpdateTime(name, h.Context().Value("time").(int64))
		finalCost.AddAssign(cost)
		cost = h.GasManager.SetGasPledgeTotal(name, totalDelta)
		finalCost.AddAssign(cost)
		cost = h.GasManager.SetGasLimit(name, limitDelta)
		finalCost.AddAssign(cost)
		cost = h.GasManager.SetGasStock(name, gasDelta)
		finalCost.AddAssign(cost)
		return finalCost, nil
	}
	cost, _ = h.GasManager.RefreshPGas(name)
	finalCost.AddAssign(cost)

	// change pledge total
	totalOld, cost := h.GasManager.GasPledgeTotal(name)
	finalCost.AddAssign(cost)
	totalNew := totalOld.Add(totalDelta)
	if totalNew.Value <= 0 {
		return finalCost, fmt.Errorf("change gasPledgeTotal failed! current: %v, delta %v", totalOld.ToString(), totalDelta.ToString())
	}
	cost = h.GasManager.SetGasPledgeTotal(name, totalNew)
	finalCost.AddAssign(cost)
	// change limit
	limitOld, cost := h.GasManager.GasLimit(name)
	finalCost.AddAssign(cost)
	limitNew := limitOld.Add(limitDelta)
	if limitNew.Value <= 0 {
		return finalCost, fmt.Errorf("change gasLimit failed! current: %v, delta %v", limitOld.ToString(), limitDelta.ToString())
	}
	cost = h.GasManager.SetGasLimit(name, limitNew)
	finalCost.AddAssign(cost)
	// change stock
	gasOld, cost := h.GasManager.GasStock(name)
	finalCost.AddAssign(cost)
	gasNew := gasOld.Add(gasDelta)
	if limitNew.LessThan(gasNew) {
		// clear the gas above the new limit.
		gasNew = limitNew
	}
	cost = h.GasManager.SetGasStock(name, gasNew)
	finalCost.AddAssign(cost)
	// change personal pledge
	pledged, cost := h.GasManager.GasPledge(name, pledger)
	finalCost.AddAssign(cost)
	newPledge := pledged.Add(totalDelta)
	if newPledge.IsZero() {
		cost = h.GasManager.DelGasPledge(name, pledger)
		finalCost.AddAssign(cost)
	} else if newPledge.IsPositive() {
		cost = h.GasManager.SetGasPledge(name, pledger, newPledge)
		finalCost.AddAssign(cost)
	} else {
		ilog.Fatalf("should not reach here pledger %v name %v pledgeAmountF %v pledged %v", pledger, name, pledgeAmountF, pledged)
	}
	return finalCost, nil
}

var (
	constructor = &abi{
		name: "constructor",
		args: []string{},
		do: func(h *host.Host, args ...interface{}) (rtn []interface{}, cost contract.Cost, err error) {
			return []interface{}{}, host.CommonErrorCost(1), nil
		},
	}
	initFunc = &abi{
		name: "init",
		args: []string{},
		do: func(h *host.Host, args ...interface{}) (rtn []interface{}, cost contract.Cost, err error) {
			return []interface{}{}, host.CommonErrorCost(1), nil
		},
	}
	pledgeGas = &abi{
		name: "pledge",
		args: []string{"string", "string", "string"},
		do: func(h *host.Host, args ...interface{}) (rtn []interface{}, cost contract.Cost, err error) {
			cost = contract.Cost0()
			pledger, ok := args[0].(string)
			cost.AddAssign(host.CommonErrorCost(1))
			if !ok || !h.IsValidAccount(pledger) {
				return nil, cost, fmt.Errorf("invalid user name %s", args[0])
			}
			gasUser, ok := args[1].(string)
			cost.AddAssign(host.CommonErrorCost(1))
			if !ok || !h.IsValidAccount(gasUser) {
				return nil, cost, fmt.Errorf("invalid user name %s", args[1])
			}
			auth, cost0 := h.RequireAuth(pledger, TransferPermission)
			cost.AddAssign(cost0)
			if !auth {
				return nil, cost, host.ErrPermissionLost
			}
			pledgeAmountStr, ok := args[2].(string)
			if !ok {
				return nil, cost, fmt.Errorf("invalid amount %s", args[2])
			}
			pledgeAmount, err := common.NewFixed(pledgeAmountStr, 8)
			cost.AddAssign(host.CommonErrorCost(1))
			if err != nil || pledgeAmount.Value <= 0 {
				return nil, cost, fmt.Errorf("invalid amount %s", args[2])
			}
			if pledgeAmount.LessThan(database.GasMinPledgePerAction) {
				return nil, cost, fmt.Errorf("min pledge num is %d", database.GasMinPledgePerAction)
			}
			cost0, err = pledge(h, pledger, gasUser, pledgeAmount)
			cost.AddAssign(cost0)
			if err != nil {
				return nil, cost, err
			}
			contractName, cost0 := h.ContractName()
			cost.AddAssign(cost0)
			_, cost0, err = h.Call("token.iost", "transfer", fmt.Sprintf(`["iost", "%v", "%v", "%v", ""]`, pledger, contractName, pledgeAmountStr))
			cost.AddAssign(cost0)
			if err != nil {
				return nil, cost, err
			}

			// generate receipt
			message, err := json.Marshal(args)
			cost.AddAssign(host.CommonOpCost(1))
			if err != nil {
				return nil, cost, err
			}
			cost0 = h.Receipt(string(message))
			cost.AddAssign(cost0)
			return []interface{}{}, cost, nil
		},
	}
	unpledgeGas = &abi{
		name: "unpledge",
		args: []string{"string", "string", "string"},
		do: func(h *host.Host, args ...interface{}) (rtn []interface{}, cost contract.Cost, err error) {
			cost = contract.Cost0()
			pledger, ok := args[0].(string)
			cost.AddAssign(host.CommonErrorCost(1))
			if !ok || !h.IsValidAccount(pledger) {
				return nil, cost, fmt.Errorf("invalid user name %s", args[1])
			}
			gasUser, ok := args[1].(string)
			cost.AddAssign(host.CommonErrorCost(1))
			if !ok || !h.IsValidAccount(gasUser) {
				return nil, cost, fmt.Errorf("invalid user name %s", args[0])
			}
			auth, cost0 := h.RequireAuth(pledger, TransferPermission)
			cost.AddAssign(cost0)
			if !auth {
				return nil, cost, host.ErrPermissionLost
			}
			unpledgeAmountStr, ok := args[2].(string)
			if !ok {
				return nil, cost, fmt.Errorf("invalid amount %s", args[2])
			}
			unpledgeAmount, err := common.NewFixed(unpledgeAmountStr, 8)
			cost.AddAssign(host.CommonErrorCost(1))
			if err != nil || unpledgeAmount.Value <= 0 {
				return nil, cost, fmt.Errorf("invalid amount %s", args[2])
			}
			if unpledgeAmount.LessThan(database.GasMinPledgePerAction) {
				return nil, cost, fmt.Errorf("min unpledge num is %d", database.GasMinPledgePerAction)
			}
			pledged, cost0 := h.GasManager.GasPledge(gasUser, pledger)
			cost.AddAssign(cost0)
			if pledged.IsZero() {
				return nil, cost, fmt.Errorf("%v did not pledge for %v", pledger, gasUser)
			}
			if pledged.LessThan(unpledgeAmount) {
				unpledgeAmount = pledged.Neg()
			}

			cost0, err = pledge(h, pledger, gasUser, unpledgeAmount.Neg())
			cost.AddAssign(cost0)
			if err != nil {
				return nil, cost, err
			}

			contractName, cost0 := h.ContractName()
			cost.AddAssign(cost0)
			freezeTime := h.Context().Value("time").(int64) + UnpledgeFreezeSeconds*1e9
			_, cost0, err = h.CallWithAuth("token.iost", "transferFreeze",
				fmt.Sprintf(`["iost", "%v", "%v", "%v", %v, ""]`, contractName, pledger, unpledgeAmount.ToString(), freezeTime))
			cost.AddAssign(cost0)

			if err != nil {
				return nil, cost, err
			}

			// generate receipt
			message, err := json.Marshal(args)
			cost.AddAssign(host.CommonOpCost(1))
			if err != nil {
				return nil, cost, err
			}
			cost0 = h.Receipt(string(message))
			cost.AddAssign(cost0)
			return []interface{}{}, cost, nil
		},
	}
	transferTGas = &abi{
		name: "transfer",
		args: []string{"string", "string", "string"},
		do: func(h *host.Host, args ...interface{}) (rtn []interface{}, cost contract.Cost, err error) {
			cost = contract.Cost0()
			from := args[0].(string)
			if !h.IsValidAccount(from) {
				return nil, cost, fmt.Errorf("invalid user name %v", from)
			}
			to := args[1].(string)
			if !h.IsValidAccount(to) {
				return nil, cost, fmt.Errorf("invalid user name %v", to)
			}
			auth, cost0 := h.RequireAuth(from, TransferPermission)
			cost.AddAssign(cost0)
			if !auth {
				return nil, cost, host.ErrPermissionLost
			}
			f, err := common.NewFixed(args[2].(string), database.GasDecimal)
			if err != nil {
				return nil, cost, fmt.Errorf("invalid gas amount %v", err)
			}
			minTransferAmount := &common.Fixed{Value: 100 * 100, Decimal: database.GasDecimal}
			if f.LessThan(minTransferAmount) {
				return nil, cost, fmt.Errorf("min transfer amount is %v", minTransferAmount.ToString())
			}
			quota, cost0 := h.TGasQuota(from)
			cost.AddAssign(cost0)
			if quota.LessThan(f) {
				return nil, cost, fmt.Errorf("transferable gas not enough %v < %v", quota.ToString(), f.ToString())
			}
			cost0 = h.ChangeTGas(from, f.Neg(), true)
			cost.AddAssign(cost0)
			cost0 = h.ChangeTGas(to, f, false)
			cost.AddAssign(cost0)

			// generate receipt
			message, err := json.Marshal(args)
			cost.AddAssign(host.CommonOpCost(1))
			if err != nil {
				return nil, cost, err
			}
			cost0 = h.Receipt(string(message))
			cost.AddAssign(cost0)
			return []interface{}{}, cost, nil
		},
	}
)
