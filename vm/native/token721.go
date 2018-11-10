package native

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/iost-official/go-iost/core/contract"
	"github.com/iost-official/go-iost/vm/host"
)

var token721ABIs map[string]*abi

// const prefix
const (
	Token721InfoMapPrefix     = "T721I"
	Token721BalanceMapPrefix  = "T721B"
	Token721IssuerMapField    = "T721issuer"
	Token721MetadataMapPrefix = "T721M"
	Token721TokensPrefix      = "T721T"
)

func init() {
	token721ABIs = make(map[string]*abi)
	register(token721ABIs, initToken721ABI)
	register(token721ABIs, createToken721ABI)
	register(token721ABIs, issueToken721ABI)
	register(token721ABIs, transferToken721ABI)
	register(token721ABIs, balanceOfToken721ABI)
	register(token721ABIs, ownerOfToken721ABI)
	register(token721ABIs, tokenOfOwnerByIndexToken721ABI)
	register(token721ABIs, tokenMetadataToken721ABI)
}

func checkToken721Exists(h *host.Host, tokenName string) (ok bool, cost contract.Cost) {
	exists, cost0 := h.MapHas(Token721InfoMapPrefix+tokenName, Token721IssuerMapField)
	return exists, cost0
}

func getToken721Balance(h *host.Host, tokenName string, from string) (balance int64, cost contract.Cost, err error) {
	balance = int64(0)
	cost = contract.Cost0()
	ok, cost0 := h.MapHas(Token721BalanceMapPrefix+from, tokenName, from)
	cost.AddAssign(cost0)
	if ok {
		tmp, cost0 := h.MapGet(Token721BalanceMapPrefix+from, tokenName, from)
		cost.AddAssign(cost0)
		balance = tmp.(int64)
	}
	return balance, cost, nil
}

func setToken721Balance(h *host.Host, tokenName string, from string, balance int64) (cost contract.Cost) {
	cost = h.MapPut(Token721BalanceMapPrefix+from, tokenName, balance, from)
	return cost

}
func getToken721Tokens(h *host.Host, tokenName string, from string) (tokens []string, tokensStr string, cost contract.Cost, err error) {
	tokensStr = "|"
	tokens = make([]string, 0, 0)
	cost = contract.Cost0()
	ok, cost0 := h.MapHas(Token721TokensPrefix+from, tokenName, from)
	cost.AddAssign(cost0)
	if ok {
		tmp, cost0 := h.MapGet(Token721TokensPrefix+from, tokenName, from)
		cost.AddAssign(cost0)
		tokensStr = tmp.(string)
		tokens = strings.Split(tokensStr, "|")
		tokens = tokens[1:]
	}
	// fmt.Println(tokensStr)
	// fmt.Println(tokens)
	return tokens, tokensStr, cost, nil
}

func addToken721Tokens(h *host.Host, tokenName string, from string, tokenID string) (cost contract.Cost, err error) {
	var tokensStr string
	_, tokensStr, cost, err = getToken721Tokens(h, tokenName, from)
	if err != nil {
		return cost, err
	}
	tokensStr += tokenID + "|"
	cost0 := h.MapPut(Token721TokensPrefix+from, tokenName, tokensStr, from)
	cost.AddAssign(cost0)
	return cost, nil
}

func delToken721Tokens(h *host.Host, tokenName string, from string, tokenID string) (cost contract.Cost, err error) {
	var tokensStr string
	_, tokensStr, cost, err = getToken721Tokens(h, tokenName, from)
	if err != nil {
		return cost, err
	}
	tokensStr = strings.Replace(tokensStr, "|"+tokenID+"|", "|", 1)
	cost0 := h.MapPut(Token721TokensPrefix+from, tokenName, tokensStr, from)
	cost.AddAssign(cost0)
	return cost, nil
}

var (
	initToken721ABI = &abi{
		name: "init",
		args: []string{},
		do: func(h *host.Host, args ...interface{}) (rtn []interface{}, cost contract.Cost, err error) {
			return []interface{}{}, host.CommonErrorCost(1), nil
		},
	}

	createToken721ABI = &abi{
		name: "create",
		args: []string{"string", "string", "number"},
		do: func(h *host.Host, args ...interface{}) (rtn []interface{}, cost contract.Cost, err error) {
			cost = contract.Cost0()
			cost.AddAssign(host.CommonOpCost(1))
			tokenName := args[0].(string)
			issuer := args[1].(string)
			totalSupply := args[2].(int64)
			// check auth
			ok, cost0 := h.RequireAuth(issuer, "token721.iost")
			cost.AddAssign(cost0)
			if !ok {
				return nil, cost, host.ErrPermissionLost
			}
			if !CheckCost(h, cost) {
				return nil, cost, host.ErrGasLimitExceeded
			}

			// check exists
			ok, cost0 = checkToken721Exists(h, tokenName)
			cost.AddAssign(cost0)
			if ok {
				return nil, cost, host.ErrTokenExists
			}

			// check valid
			if totalSupply > math.MaxInt64 {
				return nil, cost, errors.New("invalid total supply")
			}

			// put info
			cost0 = h.MapPut(Token721InfoMapPrefix+tokenName, Token721IssuerMapField, issuer)
			cost.AddAssign(cost0)
			cost0 = h.MapPut(Token721InfoMapPrefix+tokenName, TotalSupplyMapField, totalSupply, issuer)
			cost.AddAssign(cost0)
			cost0 = h.MapPut(Token721InfoMapPrefix+tokenName, SupplyMapField, int64(0), issuer)
			cost.AddAssign(cost0)

			return []interface{}{}, cost, nil
		},
	}

	issueToken721ABI = &abi{
		name: "issue",
		args: []string{"string", "string", "json"},
		do: func(h *host.Host, args ...interface{}) (rtn []interface{}, cost contract.Cost, err error) {
			cost = contract.Cost0()
			cost.AddAssign(host.CommonOpCost(1))
			tokenName := args[0].(string)
			to := args[1].(string)
			metaDateJSON := args[2].(string)

			metaDate := make(map[string]interface{})
			err = json.Unmarshal([]byte(metaDateJSON), &metaDate)
			cost.AddAssign(host.CommonOpCost(2))
			if err != nil {
				return nil, cost, err
			}

			// get token info
			ok, cost0 := checkToken721Exists(h, tokenName)
			cost.AddAssign(cost0)
			if !ok {
				return nil, cost, host.ErrTokenNotExists
			}
			issuer, cost0 := h.MapGet(Token721InfoMapPrefix+tokenName, Token721IssuerMapField)
			cost.AddAssign(cost0)
			supply, cost0 := h.MapGet(Token721InfoMapPrefix+tokenName, SupplyMapField, issuer.(string))
			cost.AddAssign(cost0)
			totalSupply, cost0 := h.MapGet(Token721InfoMapPrefix+tokenName, TotalSupplyMapField, issuer.(string))
			cost.AddAssign(cost0)
			if !CheckCost(h, cost) {
				return nil, cost, host.ErrGasLimitExceeded
			}

			// check supply
			if totalSupply.(int64)-supply.(int64) <= 0 {
				return nil, cost, errors.New("supply too much")
			}

			tokenID := strconv.FormatInt(supply.(int64), 10)
			// check auth
			ok, cost0 = h.RequireAuth(issuer.(string), "token.iost")
			cost.AddAssign(cost0)
			if !ok {
				return nil, cost, host.ErrPermissionLost
			}
			if !CheckCost(h, cost) {
				return nil, cost, host.ErrGasLimitExceeded
			}

			// set supply, set balance
			cost0 = h.MapPut(Token721InfoMapPrefix+tokenName, SupplyMapField, supply.(int64)+1, issuer.(string))
			cost.AddAssign(cost0)

			cost0 = h.MapPut(Token721InfoMapPrefix+tokenName, tokenID, to)
			cost.AddAssign(cost0)

			tbalance, cost0, err := getToken721Balance(h, tokenName, to)
			cost.AddAssign(cost0)
			if err != nil {
				return nil, cost, err
			}
			tbalance += 1
			cost0 = setToken721Balance(h, tokenName, to, tbalance)
			cost.AddAssign(cost0)

			cost0 = h.MapPut(Token721MetadataMapPrefix+tokenName, tokenID, metaDateJSON, to)
			cost.AddAssign(cost0)

			cost0, err = addToken721Tokens(h, tokenName, to, tokenID)
			cost.AddAssign(cost0)

			return []interface{}{}, cost, nil
		},
	}

	transferToken721ABI = &abi{
		name: "transfer",
		args: []string{"string", "string", "string", "string"},
		do: func(h *host.Host, args ...interface{}) (rtn []interface{}, cost contract.Cost, err error) {
			cost = contract.Cost0()
			cost.AddAssign(host.CommonOpCost(1))
			tokenName := args[0].(string)
			from := args[1].(string)
			to := args[2].(string)
			tokenID := args[3].(string)

			if from == to {
				return []interface{}{}, cost, nil
			}

			// get token info
			ok, cost0 := checkToken721Exists(h, tokenName)
			cost.AddAssign(cost0)
			if !ok {
				return nil, cost, host.ErrTokenNotExists
			}

			// check auth
			// todo handle from is contract
			ok, cost0 = h.RequireAuth(from, "transfer")
			cost.AddAssign(cost0)
			if !ok {
				return nil, cost, host.ErrPermissionLost
			}
			if !CheckCost(h, cost) {
				return nil, cost, host.ErrGasLimitExceeded
			}

			tmp, cost0 := h.MapGet(Token721InfoMapPrefix+tokenName, tokenID)
			cost.AddAssign(cost0)
			owner := tmp.(string)
			if owner != from {
				return nil, cost, host.ErrInvalidData
			}

			cost0 = h.MapPut(Token721InfoMapPrefix+tokenName, tokenID, to)
			cost.AddAssign(cost0)

			fbalance, cost0, err := getToken721Balance(h, tokenName, from)
			cost.AddAssign(cost0)
			if err != nil {
				return nil, cost, err
			}
			tbalance, cost0, err := getToken721Balance(h, tokenName, to)
			cost.AddAssign(cost0)
			if err != nil {
				return nil, cost, err
			}

			fbalance -= 1
			tbalance += 1

			cost0 = setToken721Balance(h, tokenName, from, fbalance)
			cost.AddAssign(cost0)
			cost0 = setToken721Balance(h, tokenName, to, tbalance)
			cost.AddAssign(cost0)

			metaDateJSON, cost0 := h.MapGet(Token721MetadataMapPrefix+tokenName, tokenID, from)
			cost.AddAssign(cost0)
			cost0 = h.MapDel(Token721MetadataMapPrefix+tokenName, tokenID, from)
			cost.AddAssign(cost0)
			cost0 = h.MapPut(Token721MetadataMapPrefix+tokenName, tokenID, metaDateJSON, to)
			cost.AddAssign(cost0)

			cost0, err = delToken721Tokens(h, tokenName, from, tokenID)
			cost.AddAssign(cost0)

			cost0, err = addToken721Tokens(h, tokenName, to, tokenID)
			cost.AddAssign(cost0)

			return []interface{}{}, cost, nil
		},
	}

	balanceOfToken721ABI = &abi{
		name: "balanceOf",
		args: []string{"string", "string"},
		do: func(h *host.Host, args ...interface{}) (rtn []interface{}, cost contract.Cost, err error) {
			cost = contract.Cost0()
			cost.AddAssign(host.CommonOpCost(1))
			tokenName := args[0].(string)
			to := args[1].(string)

			// check token info
			ok, cost0 := checkToken721Exists(h, tokenName)
			cost.AddAssign(cost0)
			if !ok {
				return nil, cost, host.ErrTokenNotExists
			}

			tbalance, cost0, err := getToken721Balance(h, tokenName, to)
			cost.AddAssign(cost0)
			if err != nil {
				return nil, cost, err
			}

			return []interface{}{tbalance}, cost, nil
		},
	}

	ownerOfToken721ABI = &abi{
		name: "ownerOf",
		args: []string{"string", "string"},
		do: func(h *host.Host, args ...interface{}) (rtn []interface{}, cost contract.Cost, err error) {
			cost = contract.Cost0()
			cost.AddAssign(host.CommonOpCost(1))
			tokenName := args[0].(string)
			tokenID := args[1].(string)

			// check token info
			ok, cost0 := checkToken721Exists(h, tokenName)
			cost.AddAssign(cost0)
			if !ok {
				return nil, cost, host.ErrTokenNotExists
			}

			ok, cost0 = h.MapHas(Token721InfoMapPrefix+tokenName, tokenID)
			cost.AddAssign(cost0)
			if !ok {
				return nil, cost, host.ErrTokenExists
			}
			tmp, cost0 := h.MapGet(Token721InfoMapPrefix+tokenName, tokenID)
			cost.AddAssign(cost0)
			onwer := tmp.(string)

			return []interface{}{onwer}, cost, nil
		},
	}

	tokenOfOwnerByIndexToken721ABI = &abi{
		name: "tokenOfOwnerByIndex",
		args: []string{"string", "string", "number"},
		do: func(h *host.Host, args ...interface{}) (rtn []interface{}, cost contract.Cost, err error) {
			cost = contract.Cost0()
			cost.AddAssign(host.CommonOpCost(1))
			tokenName := args[0].(string)
			owner := args[1].(string)
			index := args[2].(int64)
			ok, cost0 := checkToken721Exists(h, tokenName)
			cost.AddAssign(cost0)
			if !ok {
				return nil, cost, host.ErrTokenNotExists
			}
			tokens, _, cost0, err := getToken721Tokens(h, tokenName, owner)
			cost.AddAssign(cost0)
			if err != nil {
				return nil, cost, err
			}
			if int(index) >= len(tokens) {
				return nil, cost, errors.New("out of range")
			}

			return []interface{}{tokens[index]}, cost, nil
		},
	}

	tokenMetadataToken721ABI = &abi{
		name: "tokenMetadata",
		args: []string{"string", "string"},
		do: func(h *host.Host, args ...interface{}) (rtn []interface{}, cost contract.Cost, err error) {
			cost = contract.Cost0()
			cost.AddAssign(host.CommonOpCost(1))
			tokenName := args[0].(string)
			tokenID := args[1].(string)
			ok, cost0 := checkToken721Exists(h, tokenName)
			cost.AddAssign(cost0)
			if !ok {
				return nil, cost, host.ErrTokenNotExists
			}
			ok, cost0 = h.MapHas(Token721InfoMapPrefix+tokenName, tokenID)
			cost.AddAssign(cost0)
			if !ok {
				return nil, cost, host.ErrTokenExists
			}
			tmp, cost0 := h.MapGet(Token721InfoMapPrefix+tokenName, tokenID)
			cost.AddAssign(cost0)
			onwer := tmp.(string)

			ok, cost0 = h.MapHas(Token721MetadataMapPrefix+tokenName, tokenID, onwer)
			cost.AddAssign(cost0)
			if !ok {
				return nil, cost, host.ErrTokenExists
			}

			metaDateJSON, cost0 := h.MapGet(Token721MetadataMapPrefix+tokenName, tokenID, onwer)
			cost.AddAssign(cost0)
			return []interface{}{metaDateJSON.(string)}, cost, nil
		},
	}
)
