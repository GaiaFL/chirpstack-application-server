package loracloud

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	pb "github.com/brocaar/chirpstack-api/go/v3/as/integration"
	"github.com/brocaar/chirpstack-api/go/v3/common"
	gw "github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-application-server/internal/integration/loracloud/client/geolocation"
	"github.com/brocaar/chirpstack-application-server/internal/integration/models"
	"github.com/brocaar/chirpstack-application-server/internal/logging"
	"github.com/brocaar/lorawan"
)

// Config contains the LoRaCloud integration configuration.
type Config struct {
	Geolocation                 bool   `json:"geolocation"`
	GeolocationToken            string `json:"geolocationToken"`
	GeolocationBufferTTL        int    `json:"geolocationBufferTTL"`
	GeolocationMinBufferSize    int    `json:"geolocationMinBufferSize"`
	GeolocationTDOA             bool   `json:"geolocationTDOA"`
	GeolocationRSSI             bool   `json:"geolocationRSSI"`
	GeolocationGNSS             bool   `json:"geolocationGNSS"`
	GeolocationGNSSPayloadField string `json:"geolocationGNSSPayloadField"`
	GeolocationGNSSUseRxTime    bool   `json:"geolicationGNSSUseRxTime"`
	GeolocationWifi             bool   `json:"geolocationWifi"`
	GeolocationWifiPayloadField string `json:"geolocationWifiPayloadField"`
}

// Integration implements a LoRaCloud Integration.
type Integration struct {
	config         Config
	geolocationURI string
}

// New creates a new LoRaCloud integration.
func New(conf Config) (*Integration, error) {
	return &Integration{
		config:         conf,
		geolocationURI: "https://gls.loracloud.com",
	}, nil
}

// HandleUplinkEvent handles the Uplinkevent.
func (i *Integration) HandleUplinkEvent(ctx context.Context, ii models.Integration, vars map[string]string, pl pb.UplinkEvent) error {
	var devEUI lorawan.EUI64
	copy(devEUI[:], pl.DevEui)

	if i.config.Geolocation {
		// update and get geoloc buffer
		geolocBuffer, err := i.updateGeolocBuffer(ctx, devEUI, pl)
		if err != nil {
			return errors.Wrap(err, "update geolocation buffer error")
		}

		// do geolocation
		uplinkIDs, loc, err := i.geolocation(ctx, devEUI, geolocBuffer, pl)
		if err != nil {
			return errors.Wrap(err, "geolocation error")
		}

		// if it resolved to a location, send it to integrations
		if loc != nil {
			var fCnt uint32
			if len(uplinkIDs) == 0 {
				fCnt = pl.FCnt
			}
			if err := ii.HandleLocationEvent(ctx, vars, pb.LocationEvent{
				ApplicationId:   pl.ApplicationId,
				ApplicationName: pl.ApplicationName,
				DeviceName:      pl.DeviceName,
				DevEui:          pl.DevEui,
				Tags:            pl.Tags,
				Location:        loc,
				UplinkIds:       uplinkIDs,
				FCnt:            fCnt,
			}); err != nil {
				log.WithError(err).Error("integration/loracloud: geolocation error")
			}
		}
	}

	return nil
}

// HandleJoinEvent is not implemented.
func (i *Integration) HandleJoinEvent(ctx context.Context, _ models.Integration, vars map[string]string, pl pb.JoinEvent) error {
	return nil
}

// HandleAckEvent is not implemented.
func (i *Integration) HandleAckEvent(ctx context.Context, _ models.Integration, vars map[string]string, pl pb.AckEvent) error {
	return nil
}

// HandleErrorEvent is not implemented.
func (i *Integration) HandleErrorEvent(ctx context.Context, _ models.Integration, vars map[string]string, pl pb.ErrorEvent) error {
	return nil
}

// HandleStatusEvent is not implemented.
func (i *Integration) HandleStatusEvent(ctx context.Context, _ models.Integration, vars map[string]string, pl pb.StatusEvent) error {
	return nil
}

// HandleLocationEvent is not implemented.
func (i *Integration) HandleLocationEvent(ctx context.Context, _ models.Integration, vars map[string]string, pl pb.LocationEvent) error {
	return nil
}

// HandleTxAckEvent is not implemented.
func (i *Integration) HandleTxAckEvent(ctx context.Context, _ models.Integration, vars map[string]string, pl pb.TxAckEvent) error {
	return nil
}

// DataDownChan returns nil.
func (i *Integration) DataDownChan() chan models.DataDownPayload {
	return nil
}

// Close is not implemented.
func (i *Integration) Close() error {
	return nil
}

func (i *Integration) updateGeolocBuffer(ctx context.Context, devEUI lorawan.EUI64, pl pb.UplinkEvent) ([][]*gw.UplinkRXInfo, error) {
	// read the geoloc buffer
	geolocBuffer, err := GetGeolocBuffer(ctx, devEUI, time.Duration(i.config.GeolocationBufferTTL)*time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "get geoloc buffer error")
	}

	// if the uplink was received by at least 3 gateways, append the metadata
	// to the buffer
	if len(pl.RxInfo) >= 3 {
		geolocBuffer = append(geolocBuffer, pl.RxInfo)
	}

	// Save the buffer when there are > 0 items.
	if len(geolocBuffer) != 0 {
		if err := SaveGeolocBuffer(ctx, devEUI, geolocBuffer, time.Duration(i.config.GeolocationBufferTTL)*time.Second); err != nil {
			return nil, errors.Wrap(err, "save geoloc buffer error")
		}
	}

	return geolocBuffer, nil
}

func (i *Integration) geolocation(ctx context.Context, devEUI lorawan.EUI64, geolocBuffer [][]*gw.UplinkRXInfo, pl pb.UplinkEvent) ([][]byte, *common.Location, error) {
	if i.config.GeolocationGNSS {
		gnssPL, err := getBytesFromJSONObject(i.config.GeolocationGNSSPayloadField, pl.ObjectJson)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"dev_eui":       devEUI,
				"ctx_id":        ctx.Value(logging.ContextIDKey),
				"payload_field": i.config.GeolocationGNSSPayloadField,
			}).Error("integration/loracloud: get gnss bytes from object error")
			return nil, nil, nil
		}

		if len(gnssPL) == 0 {
			log.WithFields(log.Fields{
				"dev_eui":       devEUI,
				"ctx_id":        ctx.Value(logging.ContextIDKey),
				"payload_field": i.config.GeolocationGNSSPayloadField,
			}).Debug("integration/loracloud: no gnss bytes found in object")
		} else {
			loc, err := i.gnssLR1110Geolocation(ctx, devEUI, pl.RxInfo, gnssPL)
			return nil, loc, err
		}
	}

	if i.config.GeolocationWifi {
		wifiAPs, err := getWifiAccessPointsFromJSONObject(i.config.GeolocationWifiPayloadField, pl.ObjectJson)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"dev_eui":       devEUI,
				"ctx_id":        ctx.Value(logging.ContextIDKey),
				"payload_field": i.config.GeolocationWifiPayloadField,
			}).Error("integration/loracloud: get wifi access-points from object error")
			return nil, nil, nil
		}

		if len(wifiAPs) == 0 {
			log.WithFields(log.Fields{
				"dev_eui":       devEUI,
				"ctx_id":        ctx.Value(logging.ContextIDKey),
				"payload_field": i.config.GeolocationWifiPayloadField,
			}).Debug("integration/loracloud: no wifi access-points found in object")
		} else {
			loc, err := i.wifiTDOAGeolocation(ctx, devEUI, pl.RxInfo, wifiAPs)
			return nil, loc, err
		}
	}

	if i.config.GeolocationTDOA {
		tdoaFiltered := filterOnFineTimestamp(geolocBuffer, 3)
		if len(tdoaFiltered) == 0 || len(tdoaFiltered) < i.config.GeolocationMinBufferSize {
			log.WithFields(log.Fields{
				"dev_eui":              devEUI,
				"ctx_id":               ctx.Value(logging.ContextIDKey),
				"tdoa_suitable_frames": len(tdoaFiltered),
			}).Debug("integration/loracloud: not enough meta-data for tdoa geolocation")
		} else {
			var uplinkIDs [][]byte
			for i := range tdoaFiltered {
				for j := range tdoaFiltered[i] {
					uplinkIDs = append(uplinkIDs, tdoaFiltered[i][j].GetUplinkId())
				}
			}

			loc, err := i.tdoaGeolocation(ctx, devEUI, tdoaFiltered)
			return uplinkIDs, loc, err
		}
	}

	if i.config.GeolocationRSSI {
		if len(geolocBuffer) == 0 || len(geolocBuffer) < i.config.GeolocationMinBufferSize {
			log.WithFields(log.Fields{
				"dev_eui": devEUI,
				"ctx_id":  ctx.Value(logging.ContextIDKey),
				"frames":  len(geolocBuffer),
			}).Debug("integration/loracloud: not enough meta-data for rssi geolocation")
		} else {
			var uplinkIDs [][]byte
			for i := range geolocBuffer {
				for j := range geolocBuffer[i] {
					uplinkIDs = append(uplinkIDs, geolocBuffer[i][j].GetUplinkId())
				}
			}
			loc, err := i.rssiGeolocation(ctx, devEUI, geolocBuffer)
			return uplinkIDs, loc, err
		}
	}

	return nil, nil, nil
}

func (i *Integration) tdoaGeolocation(ctx context.Context, devEUI lorawan.EUI64, geolocBuffer [][]*gw.UplinkRXInfo) (*common.Location, error) {
	client := geolocation.New(i.geolocationURI, i.config.GeolocationToken)
	start := time.Now()

	var loc common.Location
	var err error

	if len(geolocBuffer) == 1 {
		// single-frame geoloc
		loc, err = client.TDOASingleFrame(ctx, geolocBuffer[0])
		loRaCloudAPIDuration("v2_tdoa_single").Observe(float64(time.Since(start)) / float64(time.Second))

	} else {
		// multi-frame geoloc
		loc, err = client.TDOAMultiFrame(ctx, geolocBuffer)
		loRaCloudAPIDuration("v2_tdoa_multi").Observe(float64(time.Since(start)) / float64(time.Second))
	}

	if err != nil {
		if err == geolocation.ErrNoLocation {
			return nil, nil
		}

		return nil, errors.Wrap(err, "geolocation error")
	}

	return &loc, nil
}

func (i *Integration) rssiGeolocation(ctx context.Context, devEUI lorawan.EUI64, geolocBuffer [][]*gw.UplinkRXInfo) (*common.Location, error) {
	client := geolocation.New(i.geolocationURI, i.config.GeolocationToken)
	start := time.Now()

	var loc common.Location
	var err error

	if len(geolocBuffer) == 1 {
		// single-frame geoloc
		loc, err = client.RSSISingleFrame(ctx, geolocBuffer[0])
		loRaCloudAPIDuration("v2_rssi_single").Observe(float64(time.Since(start)) / float64(time.Second))

	} else {
		// multi-frame geoloc
		loc, err = client.RSSIMultiFrame(ctx, geolocBuffer)
		loRaCloudAPIDuration("v2_rssi_multi").Observe(float64(time.Since(start)) / float64(time.Second))

	}

	if err != nil {
		if err == geolocation.ErrNoLocation {
			return nil, nil
		}

		return nil, errors.Wrap(err, "geolocation error")
	}

	return &loc, nil
}

func (i *Integration) gnssLR1110Geolocation(ctx context.Context, devEUI lorawan.EUI64, rxInfo []*gw.UplinkRXInfo, pl []byte) (*common.Location, error) {
	client := geolocation.New(i.geolocationURI, i.config.GeolocationToken)
	start := time.Now()

	loc, err := client.GNSSLR1110SingleFrame(ctx, rxInfo, i.config.GeolocationGNSSUseRxTime, pl)
	if err != nil {
		if err == geolocation.ErrNoLocation {
			return nil, nil
		}

		return nil, errors.Wrap(err, "geolocation error")
	}

	loRaCloudAPIDuration("v3_gnss_rl1110_single").Observe(float64(time.Since(start)) / float64(time.Second))

	return &loc, nil
}

func (i *Integration) wifiTDOAGeolocation(ctx context.Context, devEUI lorawan.EUI64, rxInfo []*gw.UplinkRXInfo, aps []geolocation.WifiAccessPoint) (*common.Location, error) {
	client := geolocation.New(i.geolocationURI, i.config.GeolocationToken)
	start := time.Now()

	loc, err := client.WifiTDOASingleFrame(ctx, rxInfo, aps)
	if err != nil {
		if err == geolocation.ErrNoLocation {
			return nil, nil
		}
	}

	loRaCloudAPIDuration("v2_wifi_tdoa_single").Observe(float64(time.Since(start)) / float64(time.Second))

	return &loc, nil
}

// filterOnFineTimestamp filters the given frame RXInfo slices on the presence
// of a plain fine-timestamp. Per frame it filters on the availability of
// minPerFrame.
func filterOnFineTimestamp(geolocBuffer [][]*gw.UplinkRXInfo, minPerFrame int) [][]*gw.UplinkRXInfo {
	var out [][]*gw.UplinkRXInfo

	for i := range geolocBuffer {
		var f []*gw.UplinkRXInfo

		for j := range geolocBuffer[i] {
			if geolocBuffer[i][j].GetFineTimestamp() != nil {
				f = append(f, geolocBuffer[i][j])
			}
		}

		if len(f) >= minPerFrame {
			out = append(out, f)
		}
	}

	return out
}

// getBytesFromJSONObject returns a slice of bytes from the decoded object,
// using the given name.
func getBytesFromJSONObject(field, jsonStr string) ([]byte, error) {
	if jsonStr == "" {
		return nil, nil
	}

	v := make(map[string]interface{})
	if err := json.Unmarshal([]byte(jsonStr), &v); err != nil {
		return nil, errors.Wrap(err, "unmarshal json error")
	}

	vv, ok := v[field]
	if !ok {
		return nil, nil
	}

	str, ok := vv.(string)
	if !ok {
		return nil, fmt.Errorf("expected string, got %T", vv)
	}

	b, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return nil, errors.Wrap(err, "base64 decode error")
	}

	return b, nil
}

// getWifiAccessPointsFromJSONObject returns a slice of Wifi APs from the
// decoded object, using the given name.
func getWifiAccessPointsFromJSONObject(field, jsonStr string) ([]geolocation.WifiAccessPoint, error) {
	if jsonStr == "" {
		return nil, nil
	}

	v := make(map[string]interface{})
	if err := json.Unmarshal([]byte(jsonStr), &v); err != nil {
		return nil, errors.Wrap(err, "unmarshal json error")
	}

	vv, ok := v[field]
	if !ok {
		return nil, nil
	}

	aps, ok := vv.([]interface{})
	if !ok {
		return nil, fmt.Errorf("field content must be a list of objects, got: %T", vv)
	}

	var out []geolocation.WifiAccessPoint

	for i := range aps {
		vvv, ok := aps[i].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("expected key / value map, got: %T", aps[i])
		}

		var ap geolocation.WifiAccessPoint
		bssid, ok := vvv["macAddress"].(string)
		if !ok {
			return nil, fmt.Errorf("macAddress must be a string, got: %T", vvv["macAddress"])
		}
		b, err := base64.StdEncoding.DecodeString(bssid)
		if err != nil {
			return nil, errors.Wrap(err, "base64 decode error")
		}
		copy(ap.MacAddress[:], b)

		ss, ok := vvv["signalStrength"].(float64)
		if !ok {
			return nil, fmt.Errorf("signalStrength must be a float64, got: %T", vvv["signalStrength"])
		}
		ap.SignalStrength = int(ss)
		out = append(out, ap)
	}

	return out, nil
}
