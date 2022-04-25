package autoborrow

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/exchange/binance"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

const ID = "autoborrow"

var log = logrus.WithField("strategy", ID)

func init() {
	bbgo.RegisterStrategy(ID, &Strategy{})
}

/**
- on: binance
  autoborrow:
    interval: 30m
    repayWhenDeposit: true

    # minMarginLevel for triggering auto borrow
    minMarginLevel: 1.5
    assets:
    - asset: ETH
      low: 3.0
      maxQuantityPerBorrow: 1.0
      maxTotalBorrow: 10.0
    - asset: USDT
      low: 1000.0
      maxQuantityPerBorrow: 100.0
      maxTotalBorrow: 10.0
*/

type MarginAsset struct {
	Asset                string           `json:"asset"`
	Low                  fixedpoint.Value `json:"low"`
	MaxTotalBorrow       fixedpoint.Value `json:"maxTotalBorrow"`
	MaxQuantityPerBorrow fixedpoint.Value `json:"maxQuantityPerBorrow"`
	MinQuantityPerBorrow fixedpoint.Value `json:"minQuantityPerBorrow"`
}

type Strategy struct {
	*bbgo.Notifiability

	Interval             types.Interval   `json:"interval"`
	MinMarginLevel       fixedpoint.Value `json:"minMarginLevel"`
	MaxMarginLevel       fixedpoint.Value `json:"maxMarginLevel"`
	AutoRepayWhenDeposit bool             `json:"autoRepayWhenDeposit"`

	Assets []MarginAsset `json:"assets"`

	ExchangeSession *bbgo.ExchangeSession

	marginBorrowRepay types.MarginBorrowRepay
}

func (s *Strategy) ID() string {
	return ID
}

func (s *Strategy) Subscribe(session *bbgo.ExchangeSession) {
	// session.Subscribe(types.KLineChannel, s.Symbol, types.SubscribeOptions{Interval: "1m"})
}

func (s *Strategy) checkAndBorrow(ctx context.Context) {
	if s.MinMarginLevel.IsZero() {
		return
	}

	if err := s.ExchangeSession.UpdateAccount(ctx); err != nil {
		log.WithError(err).Errorf("can not update account")
		return
	}

	minMarginLevel := s.MinMarginLevel
	account := s.ExchangeSession.GetAccount()
	curMarginLevel := account.MarginLevel

	log.Infof("current account margin level: %s margin ratio: %s, margin tolerance: %s",
		account.MarginLevel.String(),
		account.MarginRatio.String(),
		account.MarginTolerance.String(),
	)

	// if margin ratio is too low, do not borrow
	if curMarginLevel.Compare(minMarginLevel) < 0 {
		log.Infof("current margin level %f < min margin level %f, skip autoborrow", curMarginLevel.Float64(), minMarginLevel.Float64())
		return
	}

	balances := s.ExchangeSession.GetAccount().Balances()
	if len(balances) == 0 {
		log.Warn("balance is empty, skip autoborrow")
		return
	}

	for _, marginAsset := range s.Assets {
		if marginAsset.Low.IsZero() {
			log.Warnf("margin asset low balance is not set: %+v", marginAsset)
			continue
		}

		b, ok := balances[marginAsset.Asset]
		if ok {
			toBorrow := marginAsset.Low.Sub(b.Total())
			if toBorrow.Sign() < 0 {
				log.Infof("balance %f > low %f. no need to borrow asset %+v",
					b.Total().Float64(),
					marginAsset.Low.Float64(),
					marginAsset)
				continue
			}

			if !marginAsset.MaxQuantityPerBorrow.IsZero() {
				toBorrow = fixedpoint.Min(toBorrow, marginAsset.MaxQuantityPerBorrow)
			}

			if !marginAsset.MaxTotalBorrow.IsZero() {
				// check if we over borrow
				if toBorrow.Add(b.Borrowed).Compare(marginAsset.MaxTotalBorrow) > 0 {
					toBorrow = toBorrow.Sub(toBorrow.Add(b.Borrowed).Sub(marginAsset.MaxTotalBorrow))
					if toBorrow.Sign() < 0 {
						log.Warnf("margin asset %s is over borrowed, skip", marginAsset.Asset)
						continue
					}
				}
				toBorrow = fixedpoint.Min(toBorrow.Add(b.Borrowed), marginAsset.MaxTotalBorrow)
			}

			s.Notifiability.Notify(&MarginAction{
				Action:         "Borrow",
				Asset:          marginAsset.Asset,
				Amount:         toBorrow,
				MarginLevel:    curMarginLevel,
				MinMarginLevel: minMarginLevel,
			})
			log.Infof("sending borrow request %f %s", toBorrow.Float64(), marginAsset.Asset)
			s.marginBorrowRepay.BorrowMarginAsset(ctx, marginAsset.Asset, toBorrow)
		} else {
			// available balance is less than marginAsset.Low, we should trigger borrow
			toBorrow := marginAsset.Low

			if !marginAsset.MaxQuantityPerBorrow.IsZero() {
				toBorrow = fixedpoint.Min(toBorrow, marginAsset.MaxQuantityPerBorrow)
			}

			s.Notifiability.Notify(&MarginAction{
				Action:         "Borrow",
				Asset:          marginAsset.Asset,
				Amount:         toBorrow,
				MarginLevel:    curMarginLevel,
				MinMarginLevel: minMarginLevel,
			})

			log.Infof("sending borrow request %f %s", toBorrow.Float64(), marginAsset.Asset)
			s.marginBorrowRepay.BorrowMarginAsset(ctx, marginAsset.Asset, toBorrow)
		}
	}
}

func (s *Strategy) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.checkAndBorrow(ctx)
	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			s.checkAndBorrow(ctx)

		}
	}
}

func (s *Strategy) handleBalanceUpdate(balances types.BalanceMap) {
	if s.MinMarginLevel.IsZero() {
		return
	}

	if s.ExchangeSession.GetAccount().MarginLevel.Compare(s.MinMarginLevel) > 0 {
		return
	}

	for _, b := range balances {
		if b.Available.IsZero() && b.Borrowed.IsZero() {
			continue
		}
	}
}

func (s *Strategy) handleBinanceBalanceUpdateEvent(event *binance.BalanceUpdateEvent) {
	if s.MinMarginLevel.IsZero() {
		return
	}

	if s.ExchangeSession.GetAccount().MarginLevel.Compare(s.MinMarginLevel) > 0 {
		return
	}

	delta := fixedpoint.MustNewFromString(event.Delta)

	// ignore outflow
	if delta.Sign() < 0 {
		return
	}

	minMarginLevel := s.MinMarginLevel
	curMarginLevel := s.ExchangeSession.GetAccount().MarginLevel

	if b, ok := s.ExchangeSession.GetAccount().Balance(event.Asset); ok {
		if b.Available.IsZero() || b.Borrowed.IsZero() {
			return
		}

		toRepay := b.Available
		s.Notifiability.Notify(&MarginAction{
			Action:         "Borrow",
			Asset:          b.Currency,
			Amount:         toRepay,
			MarginLevel:    curMarginLevel,
			MinMarginLevel: minMarginLevel,
		})
		if err := s.marginBorrowRepay.RepayMarginAsset(context.Background(), event.Asset, toRepay); err != nil {
			log.WithError(err).Errorf("margin repay error")
		}
	}
}

type MarginAction struct {
	Action         string
	Asset          string
	Amount         fixedpoint.Value
	MarginLevel    fixedpoint.Value
	MinMarginLevel fixedpoint.Value
}

func (a *MarginAction) SlackAttachment() slack.Attachment {
	return slack.Attachment{
		Title: fmt.Sprintf("%s %s %s", a.Action, a.Amount, a.Asset),
		Color: "warning",
		Fields: []slack.AttachmentField{
			{
				Title: "Action",
				Value: a.Action,
				Short: true,
			},
			{
				Title: "Asset",
				Value: a.Asset,
				Short: true,
			},
			{
				Title: "Amount",
				Value: a.Amount.String(),
				Short: true,
			},
			{
				Title: "Current Margin Level",
				Value: a.MarginLevel.String(),
				Short: true,
			},
			{
				Title: "Min Margin Level",
				Value: a.MinMarginLevel.String(),
				Short: true,
			},
		},
	}
}

// This strategy simply spent all available quote currency to buy the symbol whenever kline gets closed
func (s *Strategy) Run(ctx context.Context, orderExecutor bbgo.OrderExecutor, session *bbgo.ExchangeSession) error {
	if s.MinMarginLevel.IsZero() {
		log.Warnf("minMarginLevel is 0, you should configure this minimal margin ratio for controlling the liquidation risk")
	}

	s.ExchangeSession = session

	marginBorrowRepay, ok := session.Exchange.(types.MarginBorrowRepay)
	if !ok {
		return fmt.Errorf("exchange %s does not implement types.MarginBorrowRepay", session.ExchangeName)
	}

	s.marginBorrowRepay = marginBorrowRepay

	if s.AutoRepayWhenDeposit {
		binanceStream, ok := session.UserDataStream.(*binance.Stream)
		if ok {
			binanceStream.OnBalanceUpdateEvent(s.handleBinanceBalanceUpdateEvent)
		} else {
			session.UserDataStream.OnBalanceUpdate(s.handleBalanceUpdate)
		}
	}

	go s.run(ctx, s.Interval.Duration())
	return nil
}