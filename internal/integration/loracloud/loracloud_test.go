package loracloud

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	pb "github.com/brocaar/chirpstack-api/go/v3/as/integration"
	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-application-server/internal/integration/loracloud/client/geolocation"
	"github.com/brocaar/chirpstack-application-server/internal/integration/mock"
	"github.com/brocaar/chirpstack-application-server/internal/storage"
	"github.com/brocaar/chirpstack-application-server/internal/test"
	"github.com/brocaar/lorawan"
)

type LoRaCloudTestSuite struct {
	suite.Suite

	apiResponse string
	apiRequest  string
	server      *httptest.Server
	integration *mock.Integration
	loraCloud   *Integration
}

func (ts *LoRaCloudTestSuite) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	assert.NoError(storage.Setup(conf))

	ts.server = httptest.NewServer(http.HandlerFunc(ts.apiHandler))
	ts.integration = mock.New()
	ts.loraCloud, _ = New(Config{})
	ts.loraCloud.geolocationURI = ts.server.URL
}

func (ts *LoRaCloudTestSuite) TearDownSuite() {
	ts.server.Close()
}

func (ts *LoRaCloudTestSuite) TestHandleUplinkEvent() {
	nowPB := ptypes.TimestampNow()
	altitude := float64(3.333)

	ts.T().Run("Geolocation", func(t *testing.T) {
		tests := []struct {
			name                string
			config              Config
			geolocBuffer        [][]*gw.UplinkRXInfo
			uplinkEvent         pb.UplinkEvent
			geolocationResponse interface{}

			expectedGeolocationRequest interface{}
			expectedLocationEvent      *pb.LocationEvent
		}{
			{
				name: "geolocation disabled",
				config: Config{
					Geolocation: false,
				},
				uplinkEvent: pb.UplinkEvent{
					ApplicationId:   1,
					ApplicationName: "test-app",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					DeviceName:      "test-device",
					RxInfo: []*gw.UplinkRXInfo{
						{
							UplinkId:  []byte{1},
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Time:      nowPB,
							Rssi:      1,
							LoraSnr:   1.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 111,
									},
								},
							},
						},
						{
							UplinkId:  []byte{2},
							GatewayId: []byte{2, 2, 2, 2, 2, 2, 2, 2},
							Time:      nowPB,
							Rssi:      2,
							LoraSnr:   2.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 222,
									},
								},
							},
						},
						{
							UplinkId:  []byte{3},
							GatewayId: []byte{3, 3, 3, 3, 3, 3, 3, 3},
							Time:      nowPB,
							Rssi:      3,
							LoraSnr:   3.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 333,
									},
								},
							},
						},
					},
				},
			},
			{
				name: "geolocation enabled, single TDOA",
				config: Config{
					Geolocation:     true,
					GeolocationTDOA: true,
					GeolocationRSSI: false,
				},
				geolocationResponse: &geolocation.Response{
					Result: &geolocation.LocationResult{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Accuracy:  10,
					},
				},
				expectedGeolocationRequest: &geolocation.TDOASingleFrameRequest{
					LoRaWAN: []geolocation.UplinkTDOA{
						{
							GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
							RSSI:      1,
							SNR:       1.1,
							TOA:       111,
							AntennaLocation: geolocation.AntennaLocation{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
						},
						{
							GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
							RSSI:      2,
							SNR:       2.1,
							TOA:       222,
							AntennaLocation: geolocation.AntennaLocation{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
						},
						{
							GatewayID: lorawan.EUI64{3, 3, 3, 3, 3, 3, 3, 3},
							RSSI:      3,
							SNR:       3.1,
							TOA:       333,
							AntennaLocation: geolocation.AntennaLocation{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
						},
					},
				},
				expectedLocationEvent: &pb.LocationEvent{
					ApplicationName: "test-app",
					ApplicationId:   1,
					DeviceName:      "test-device",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					UplinkIds:       [][]byte{{1}, {2}, {3}},
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER_TDOA,
						Accuracy:  10,
					},
				},
				uplinkEvent: pb.UplinkEvent{
					ApplicationId:   1,
					ApplicationName: "test-app",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					DeviceName:      "test-device",
					RxInfo: []*gw.UplinkRXInfo{
						{
							UplinkId:  []byte{1},
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Time:      nowPB,
							Rssi:      1,
							LoraSnr:   1.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 111,
									},
								},
							},
						},
						{
							UplinkId:  []byte{2},
							GatewayId: []byte{2, 2, 2, 2, 2, 2, 2, 2},
							Time:      nowPB,
							Rssi:      2,
							LoraSnr:   2.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 222,
									},
								},
							},
						},
						{
							UplinkId:  []byte{3},
							GatewayId: []byte{3, 3, 3, 3, 3, 3, 3, 3},
							Time:      nowPB,
							Rssi:      3,
							LoraSnr:   3.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 333,
									},
								},
							},
						},
					},
				},
			},
			{
				name: "geolocation enabled, single RSSI",
				config: Config{
					Geolocation:     true,
					GeolocationTDOA: false,
					GeolocationRSSI: true,
				},
				geolocationResponse: &geolocation.Response{
					Result: &geolocation.LocationResult{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Accuracy:  10,
					},
				},
				expectedGeolocationRequest: &geolocation.RSSISingleFrameRequest{
					LoRaWAN: []geolocation.UplinkRSSI{
						{
							GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
							RSSI:      1,
							SNR:       1.1,
							AntennaLocation: geolocation.AntennaLocation{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
						},
						{
							GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
							RSSI:      2,
							SNR:       2.1,
							AntennaLocation: geolocation.AntennaLocation{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
						},
						{
							GatewayID: lorawan.EUI64{3, 3, 3, 3, 3, 3, 3, 3},
							RSSI:      3,
							SNR:       3.1,
							AntennaLocation: geolocation.AntennaLocation{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
						},
					},
				},
				expectedLocationEvent: &pb.LocationEvent{
					ApplicationName: "test-app",
					ApplicationId:   1,
					DeviceName:      "test-device",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					UplinkIds:       [][]byte{{1}, {2}, {3}},
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER_RSSI,
						Accuracy:  10,
					},
				},
				uplinkEvent: pb.UplinkEvent{
					ApplicationId:   1,
					ApplicationName: "test-app",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					DeviceName:      "test-device",
					RxInfo: []*gw.UplinkRXInfo{
						{
							UplinkId:  []byte{1},
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Time:      nowPB,
							Rssi:      1,
							LoraSnr:   1.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 111,
									},
								},
							},
						},
						{
							UplinkId:  []byte{2},
							GatewayId: []byte{2, 2, 2, 2, 2, 2, 2, 2},
							Time:      nowPB,
							Rssi:      2,
							LoraSnr:   2.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 222,
									},
								},
							},
						},
						{
							UplinkId:  []byte{3},
							GatewayId: []byte{3, 3, 3, 3, 3, 3, 3, 3},
							Time:      nowPB,
							Rssi:      3,
							LoraSnr:   3.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 333,
									},
								},
							},
						},
					},
				},
			},
			{
				name: "geolocation enabled, fallback to RSSI (only two fine-timestamps)",
				config: Config{
					Geolocation:     true,
					GeolocationTDOA: true,
					GeolocationRSSI: true,
				},
				geolocationResponse: &geolocation.Response{
					Result: &geolocation.LocationResult{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Accuracy:  10,
					},
				},
				expectedGeolocationRequest: &geolocation.RSSISingleFrameRequest{
					LoRaWAN: []geolocation.UplinkRSSI{
						{
							GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
							RSSI:      1,
							SNR:       1.1,
							AntennaLocation: geolocation.AntennaLocation{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
						},
						{
							GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
							RSSI:      2,
							SNR:       2.1,
							AntennaLocation: geolocation.AntennaLocation{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
						},
						{
							GatewayID: lorawan.EUI64{3, 3, 3, 3, 3, 3, 3, 3},
							RSSI:      3,
							SNR:       3.1,
							AntennaLocation: geolocation.AntennaLocation{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
						},
					},
				},
				expectedLocationEvent: &pb.LocationEvent{
					ApplicationName: "test-app",
					ApplicationId:   1,
					DeviceName:      "test-device",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					UplinkIds:       [][]byte{{1}, {2}, {3}},
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER_RSSI,
						Accuracy:  10,
					},
				},
				uplinkEvent: pb.UplinkEvent{
					ApplicationId:   1,
					ApplicationName: "test-app",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					DeviceName:      "test-device",
					RxInfo: []*gw.UplinkRXInfo{
						{
							UplinkId:  []byte{1},
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Time:      nowPB,
							Rssi:      1,
							LoraSnr:   1.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
						},
						{
							UplinkId:  []byte{2},
							GatewayId: []byte{2, 2, 2, 2, 2, 2, 2, 2},
							Time:      nowPB,
							Rssi:      2,
							LoraSnr:   2.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 222,
									},
								},
							},
						},
						{
							UplinkId:  []byte{3},
							GatewayId: []byte{3, 3, 3, 3, 3, 3, 3, 3},
							Time:      nowPB,
							Rssi:      3,
							LoraSnr:   3.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 333,
									},
								},
							},
						},
					},
				},
			},
			{
				name: "geoloc buffer too small",
				config: Config{
					Geolocation:              true,
					GeolocationTDOA:          true,
					GeolocationMinBufferSize: 2,
					GeolocationBufferTTL:     60,
				},
				uplinkEvent: pb.UplinkEvent{
					ApplicationId:   1,
					ApplicationName: "test-app",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					DeviceName:      "test-device",
					RxInfo: []*gw.UplinkRXInfo{
						{
							UplinkId:  []byte{1},
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Time:      nowPB,
							Rssi:      1,
							LoraSnr:   1.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 111,
									},
								},
							},
						},
						{
							UplinkId:  []byte{2},
							GatewayId: []byte{2, 2, 2, 2, 2, 2, 2, 2},
							Time:      nowPB,
							Rssi:      2,
							LoraSnr:   2.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 222,
									},
								},
							},
						},
						{
							UplinkId:  []byte{3},
							GatewayId: []byte{3, 3, 3, 3, 3, 3, 3, 3},
							Time:      nowPB,
							Rssi:      3,
							LoraSnr:   3.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 333,
									},
								},
							},
						},
					},
				},
			},
			{
				name: "geolocation with buffer - TDOA",
				config: Config{
					Geolocation:              true,
					GeolocationTDOA:          true,
					GeolocationRSSI:          false,
					GeolocationMinBufferSize: 2,
					GeolocationBufferTTL:     60,
				},
				geolocationResponse: &geolocation.Response{
					Result: &geolocation.LocationResult{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Accuracy:  10,
					},
				},
				expectedGeolocationRequest: &geolocation.TDOAMultiFrameRequest{
					LoRaWAN: [][]geolocation.UplinkTDOA{
						{
							{
								GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
								RSSI:      1,
								SNR:       1.1,
								TOA:       444,
								AntennaLocation: geolocation.AntennaLocation{
									Latitude:  1.111,
									Longitude: 2.222,
									Altitude:  3.333,
								},
							},
							{
								GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
								RSSI:      2,
								SNR:       2.1,
								TOA:       555,
								AntennaLocation: geolocation.AntennaLocation{
									Latitude:  1.111,
									Longitude: 2.222,
									Altitude:  3.333,
								},
							},
							{
								GatewayID: lorawan.EUI64{3, 3, 3, 3, 3, 3, 3, 3},
								RSSI:      3,
								SNR:       3.1,
								TOA:       666,
								AntennaLocation: geolocation.AntennaLocation{
									Latitude:  1.111,
									Longitude: 2.222,
									Altitude:  3.333,
								},
							},
						},
						{
							{
								GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
								RSSI:      1,
								SNR:       1.1,
								TOA:       111,
								AntennaLocation: geolocation.AntennaLocation{
									Latitude:  1.111,
									Longitude: 2.222,
									Altitude:  3.333,
								},
							},
							{
								GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
								RSSI:      2,
								SNR:       2.1,
								TOA:       222,
								AntennaLocation: geolocation.AntennaLocation{
									Latitude:  1.111,
									Longitude: 2.222,
									Altitude:  3.333,
								},
							},
							{
								GatewayID: lorawan.EUI64{3, 3, 3, 3, 3, 3, 3, 3},
								RSSI:      3,
								SNR:       3.1,
								TOA:       333,
								AntennaLocation: geolocation.AntennaLocation{
									Latitude:  1.111,
									Longitude: 2.222,
									Altitude:  3.333,
								},
							},
						},
					},
				},
				expectedLocationEvent: &pb.LocationEvent{
					ApplicationName: "test-app",
					ApplicationId:   1,
					DeviceName:      "test-device",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					UplinkIds:       [][]byte{{4}, {5}, {6}, {1}, {2}, {3}},
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER_TDOA,
						Accuracy:  10,
					},
				},
				geolocBuffer: [][]*gw.UplinkRXInfo{
					{
						{
							UplinkId:  []byte{4},
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Time:      nowPB,
							Rssi:      1,
							LoraSnr:   1.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 444,
									},
								},
							},
						},
						{
							UplinkId:  []byte{5},
							GatewayId: []byte{2, 2, 2, 2, 2, 2, 2, 2},
							Time:      nowPB,
							Rssi:      2,
							LoraSnr:   2.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 555,
									},
								},
							},
						},
						{
							UplinkId:  []byte{6},
							GatewayId: []byte{3, 3, 3, 3, 3, 3, 3, 3},
							Time:      nowPB,
							Rssi:      3,
							LoraSnr:   3.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 666,
									},
								},
							},
						},
					},
				},
				uplinkEvent: pb.UplinkEvent{
					ApplicationId:   1,
					ApplicationName: "test-app",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					DeviceName:      "test-device",
					RxInfo: []*gw.UplinkRXInfo{
						{
							UplinkId:  []byte{1},
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Time:      nowPB,
							Rssi:      1,
							LoraSnr:   1.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 111,
									},
								},
							},
						},
						{
							UplinkId:  []byte{2},
							GatewayId: []byte{2, 2, 2, 2, 2, 2, 2, 2},
							Time:      nowPB,
							Rssi:      2,
							LoraSnr:   2.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 222,
									},
								},
							},
						},
						{
							UplinkId:  []byte{3},
							GatewayId: []byte{3, 3, 3, 3, 3, 3, 3, 3},
							Time:      nowPB,
							Rssi:      3,
							LoraSnr:   3.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 333,
									},
								},
							},
						},
					},
				},
			},
			{
				name: "geolocation with buffer - RSSI",
				config: Config{
					Geolocation:              true,
					GeolocationTDOA:          false,
					GeolocationRSSI:          true,
					GeolocationMinBufferSize: 2,
					GeolocationBufferTTL:     60,
				},
				geolocationResponse: &geolocation.Response{
					Result: &geolocation.LocationResult{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Accuracy:  10,
					},
				},
				expectedGeolocationRequest: &geolocation.RSSIMultiFrameRequest{
					LoRaWAN: [][]geolocation.UplinkRSSI{
						{
							{
								GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
								RSSI:      1,
								SNR:       1.1,
								AntennaLocation: geolocation.AntennaLocation{
									Latitude:  1.111,
									Longitude: 2.222,
									Altitude:  3.333,
								},
							},
							{
								GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
								RSSI:      2,
								SNR:       2.1,
								AntennaLocation: geolocation.AntennaLocation{
									Latitude:  1.111,
									Longitude: 2.222,
									Altitude:  3.333,
								},
							},
							{
								GatewayID: lorawan.EUI64{3, 3, 3, 3, 3, 3, 3, 3},
								RSSI:      3,
								SNR:       3.1,
								AntennaLocation: geolocation.AntennaLocation{
									Latitude:  1.111,
									Longitude: 2.222,
									Altitude:  3.333,
								},
							},
						},
						{
							{
								GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
								RSSI:      1,
								SNR:       1.1,
								AntennaLocation: geolocation.AntennaLocation{
									Latitude:  1.111,
									Longitude: 2.222,
									Altitude:  3.333,
								},
							},
							{
								GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
								RSSI:      2,
								SNR:       2.1,
								AntennaLocation: geolocation.AntennaLocation{
									Latitude:  1.111,
									Longitude: 2.222,
									Altitude:  3.333,
								},
							},
							{
								GatewayID: lorawan.EUI64{3, 3, 3, 3, 3, 3, 3, 3},
								RSSI:      3,
								SNR:       3.1,
								AntennaLocation: geolocation.AntennaLocation{
									Latitude:  1.111,
									Longitude: 2.222,
									Altitude:  3.333,
								},
							},
						},
					},
				},
				expectedLocationEvent: &pb.LocationEvent{
					ApplicationName: "test-app",
					ApplicationId:   1,
					DeviceName:      "test-device",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					UplinkIds:       [][]byte{{4}, {5}, {6}, {1}, {2}, {3}},
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER_RSSI,
						Accuracy:  10,
					},
				},
				geolocBuffer: [][]*gw.UplinkRXInfo{
					{
						{
							UplinkId:  []byte{4},
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Time:      nowPB,
							Rssi:      1,
							LoraSnr:   1.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 444,
									},
								},
							},
						},
						{
							UplinkId:  []byte{5},
							GatewayId: []byte{2, 2, 2, 2, 2, 2, 2, 2},
							Time:      nowPB,
							Rssi:      2,
							LoraSnr:   2.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 555,
									},
								},
							},
						},
						{
							UplinkId:  []byte{6},
							GatewayId: []byte{3, 3, 3, 3, 3, 3, 3, 3},
							Time:      nowPB,
							Rssi:      3,
							LoraSnr:   3.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 666,
									},
								},
							},
						},
					},
				},
				uplinkEvent: pb.UplinkEvent{
					ApplicationId:   1,
					ApplicationName: "test-app",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					DeviceName:      "test-device",
					RxInfo: []*gw.UplinkRXInfo{
						{
							UplinkId:  []byte{1},
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Time:      nowPB,
							Rssi:      1,
							LoraSnr:   1.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 111,
									},
								},
							},
						},
						{
							UplinkId:  []byte{2},
							GatewayId: []byte{2, 2, 2, 2, 2, 2, 2, 2},
							Time:      nowPB,
							Rssi:      2,
							LoraSnr:   2.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 222,
									},
								},
							},
						},
						{
							UplinkId:  []byte{3},
							GatewayId: []byte{3, 3, 3, 3, 3, 3, 3, 3},
							Time:      nowPB,
							Rssi:      3,
							LoraSnr:   3.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 333,
									},
								},
							},
						},
					},
				},
			},
			{
				name: "gnss geolocation",
				config: Config{
					Geolocation:                 true,
					GeolocationGNSS:             true,
					GeolocationGNSSPayloadField: "lr1110_gnss",
				},
				geolocationResponse: &geolocation.V3Response{
					Result: &geolocation.LocationSolverResult{
						LLH:      []float64{1.123, 2.123, 3.123},
						Accuracy: 10,
					},
				},
				expectedGeolocationRequest: &geolocation.GNSSLR1110SingleFrameRequest{
					Payload:            geolocation.HEXBytes([]byte{1, 2, 3}),
					GNSSAssistPosition: []float64{1.111, 2.222},
					GNSSAssistAltitude: &altitude,
				},
				expectedLocationEvent: &pb.LocationEvent{
					ApplicationName: "test-app",
					ApplicationId:   1,
					DeviceName:      "test-device",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER_GNSS,
						Accuracy:  10,
					},
				},
				uplinkEvent: pb.UplinkEvent{
					ApplicationId:   1,
					ApplicationName: "test-app",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					DeviceName:      "test-device",
					ObjectJson:      `{"lr1110_gnss": "AQID"}`,
					RxInfo: []*gw.UplinkRXInfo{
						{
							UplinkId:  []byte{1},
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Time:      nowPB,
							Rssi:      1,
							LoraSnr:   1.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 111,
									},
								},
							},
						},
					},
				},
			},
			{
				name: "gnss geolocation, no payload",
				config: Config{
					Geolocation:                 true,
					GeolocationGNSS:             true,
					GeolocationGNSSPayloadField: "lr1110_gnss",
				},
				uplinkEvent: pb.UplinkEvent{
					ApplicationId:   1,
					ApplicationName: "test-app",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					DeviceName:      "test-device",
					ObjectJson:      `{"different_field": "AQID"}`,
					RxInfo: []*gw.UplinkRXInfo{
						{
							UplinkId:  []byte{1},
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Time:      nowPB,
							Rssi:      1,
							LoraSnr:   1.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 111,
									},
								},
							},
						},
					},
				},
			},
			{
				name: "wifi geolocation",
				config: Config{
					Geolocation:                 true,
					GeolocationWifi:             true,
					GeolocationWifiPayloadField: "wifi_aps",
				},
				geolocationResponse: &geolocation.Response{
					Result: &geolocation.LocationResult{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Accuracy:  10,
					},
				},
				expectedGeolocationRequest: &geolocation.WifiTDOASingleFrameRequest{
					LoRaWAN: []geolocation.UplinkTDOA{
						{
							GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
							RSSI:      1,
							SNR:       1.1,
							AntennaLocation: geolocation.AntennaLocation{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
						},
					},
					WifiAccessPoints: []geolocation.WifiAccessPoint{
						{
							MacAddress:     geolocation.BSSID{1, 1, 1, 1, 1, 1},
							SignalStrength: -10,
						},
						{
							MacAddress:     geolocation.BSSID{2, 2, 2, 2, 2, 2},
							SignalStrength: -20,
						},
						{
							MacAddress:     geolocation.BSSID{3, 3, 3, 3, 3, 3},
							SignalStrength: -30,
						},
					},
				},
				expectedLocationEvent: &pb.LocationEvent{
					ApplicationName: "test-app",
					ApplicationId:   1,
					DeviceName:      "test-device",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER_WIFI,
						Accuracy:  10,
					},
				},
				uplinkEvent: pb.UplinkEvent{
					ApplicationId:   1,
					ApplicationName: "test-app",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					DeviceName:      "test-device",
					ObjectJson: `{
						"wifi_aps": [
							{"macAddress": "AQEBAQEB", "signalStrength": -10},
							{"macAddress": "AgICAgIC", "signalStrength": -20},
							{"macAddress": "AwMDAwMD", "signalStrength": -30}
						]
					}`,
					RxInfo: []*gw.UplinkRXInfo{
						{
							UplinkId:  []byte{1},
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Time:      nowPB,
							Rssi:      1,
							LoraSnr:   1.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
						},
					},
				},
			},
			{
				name: "wifi geolocation, no payload",
				config: Config{
					Geolocation:                 true,
					GeolocationWifi:             true,
					GeolocationWifiPayloadField: "wifi_aps",
				},
				uplinkEvent: pb.UplinkEvent{
					ApplicationId:   1,
					ApplicationName: "test-app",
					DevEui:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					DeviceName:      "test-device",
					RxInfo: []*gw.UplinkRXInfo{
						{
							UplinkId:  []byte{1},
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Time:      nowPB,
							Rssi:      1,
							LoraSnr:   1.1,
							Location: &common.Location{
								Latitude:  1.111,
								Longitude: 2.222,
								Altitude:  3.333,
							},
						},
					},
				},
			},
		}

		for _, tst := range tests {
			t.Run(tst.name, func(t *testing.T) {
				assert := require.New(t)
				storage.RedisClient().FlushAll()

				var devEUI lorawan.EUI64
				copy(devEUI[:], tst.uplinkEvent.DevEui)

				// set integration config
				ts.loraCloud.config = tst.config
				ts.apiRequest = ""

				// set geloc buffer
				assert.NoError(SaveGeolocBuffer(context.Background(), devEUI, tst.geolocBuffer, time.Duration(tst.config.GeolocationBufferTTL)*time.Second))

				// set api response
				if tst.geolocationResponse != nil {
					b, err := json.Marshal(tst.geolocationResponse)
					assert.NoError(err)
					ts.apiResponse = string(b)
				} else {
					ts.apiResponse = ""
				}

				// call LoRaCloud method
				assert.NoError(ts.loraCloud.HandleUplinkEvent(context.Background(), ts.integration, nil, tst.uplinkEvent))

				// assert request
				if tst.expectedGeolocationRequest != nil {
					b, err := json.Marshal(tst.expectedGeolocationRequest)
					assert.NoError(err)
					assert.Equal(string(b), ts.apiRequest)
				} else {
					assert.Equal("", ts.apiRequest)
				}

				// assert locationEvent
				if tst.expectedLocationEvent != nil {
					pl := <-ts.integration.SendLocationNotificationChan
					assert.Equal(*tst.expectedLocationEvent, pl)
				} else {
					assert.Len(ts.integration.SendLocationNotificationChan, 0)
				}
			})
		}
	})
}

func (ts *LoRaCloudTestSuite) apiHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	ts.apiRequest = string(b)
	w.Write([]byte(ts.apiResponse))
}

func TestLoRaCloud(t *testing.T) {
	suite.Run(t, new(LoRaCloudTestSuite))
}
