package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/vpx"

	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v4"
	"github.com/sourcegraph/jsonrpc2"
	_ "github.com/pion/mediadevices/pkg/driver/camera"     // This is required to register camera adapter
	_ "github.com/pion/mediadevices/pkg/driver/microphone" // This is required to register microphone adapter
)

type Candidate struct {
	Target    int                   `json:"target"`
	Candidate *webrtc.ICECandidate `json:"candidate"`
}

type ResponseCandidate struct {
	Target    int                   `json:"target"`
	Candidate *webrtc.ICECandidateInit `json:"candidate"`
}

type SendOffer struct{
	SID string 				`json:"sid"`
	Offer *webrtc.SessionDescription `json:"offer"`
}

type SendAnswer struct {
	SID string   		`json:"sid"`
	Answer *webrtc.SessionDescription 		`json:"answer"`
}

// trickle response received from sfu
type TrickleResponse struct{
	Params ResponseCandidate `json:"params"`
	Method string `json:"method"`
}

// response received from sfu over websockets
type Response struct{
	Params *webrtc.SessionDescription  `json:"params"`
	Result *webrtc.SessionDescription	`json:"result"`
	Method string						`json:"method"`
	Id 	uint64		`json:"id"`
}


var addr string
var peerConnection *webrtc.PeerConnection
var connectionID uint64
var remoteDescription *webrtc.SessionDescription


func main(){
	flag.StringVar(&addr, "a", "localhost:7000", "address to use")
	flag.Parse();

	u := url.URL{Scheme: "ws", Host: addr, Path: "/ws"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)

	if err != nil{
		log.Fatal("dial:", err)
	}

	defer c.Close()

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
		SDPSemantics: webrtc.SDPSemanticsUnifiedPlanWithFallback,
	}

	mediaEngine := webrtc.MediaEngine{}

	vpxparams, err := vpx.NewVP8Params()
	if err != nil {
		panic(err)
	}

	vpxparams.BitRate = 500_000

	codecSelector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&vpxparams),
	)
	
	codecSelector.Populate(&mediaEngine)

	api := webrtc.NewAPI(webrtc.WithMediaEngine(&mediaEngine))

	peerConnection, err = api.NewPeerConnection(config)

	if err != nil {
		panic(err)
	}

	done := make(chan struct{})

	go readMessage(c, done)

	fmt.Println(mediadevices.EnumerateDevices())


	s, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(mtc *mediadevices.MediaTrackConstraints) {
			mtc.FrameFormat = prop.FrameFormat(frame.FormatYUY2)// raw format coz vp8 needs raw input
			mtc.Width = prop.Int(640)
			mtc.Height = prop.Int(480)
		},
		Codec: codecSelector,
	})

	if err != nil {
		panic(err)
	}

	for _, track := range s.GetTracks(){
		track.OnEnded(func(err error){
			fmt.Printf("track with Id: %s ended with error %v", track.ID(), err)
		})

		_, err = peerConnection.AddTransceiverFromTrack(track, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionSendonly,
		})

		if err != nil{
			panic(err)
		}
	}

	offer, err := peerConnection.CreateOffer(nil)

	err = peerConnection.SetLocalDescription(offer)

	if err != nil{
		panic(err)
	}

	peerConnection.OnICECandidate(func (candidate *webrtc.ICECandidate){
		if candidate != nil {
			candidateJSON, err := json.Marshal(&Candidate{
				Candidate: candidate,
				Target: 0,
			})

			if err != nil {
				log.Fatal(err)
			}

			params := (*json.RawMessage)(&candidateJSON)

			message := &jsonrpc2.Request{
				Method: "trickle",
				Params: params,
			}

			reqBodyBytes := new (bytes.Buffer)
			json.NewEncoder(reqBodyBytes).Encode(message)

			reqMessageBytes := reqBodyBytes.Bytes()
			c.WriteMessage(websocket.TextMessage, reqMessageBytes)
		}
	})

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState){
		log.Printf("webrtc connection state has been changed %s", connectionState.String())
	})

	offerJson, err := json.Marshal(&SendOffer{
		Offer: peerConnection.LocalDescription(),
		SID: "test room",
	})

	params := (*json.RawMessage)(&offerJson)

	connectionUUID := uuid.New()
	connectionID = uint64(connectionUUID.ID())

	offerMessage := &jsonrpc2.Request{
		Method: "join",
		Params: params,
		ID: jsonrpc2.ID{
			IsString: false,
			Str: "",
			Num: connectionID,
		},
	}

	reqBodyBytes := new(bytes.Buffer)
	json.NewEncoder(reqBodyBytes).Encode(offerMessage)

	msgBytes := reqBodyBytes.Bytes()
	c.WriteMessage(websocket.TextMessage, msgBytes)

	<-done
}

func readMessage(c *websocket.Conn, done chan struct{}){
	defer close(done)	

	for{
		_, message, err:= c.ReadMessage()
		
		if err != nil || err == io.EOF {
			log.Fatal("Error:", err)
			break
		}

		fmt.Printf("recv: %s", message);

		var response Response
		json.Unmarshal(message, &response)

		if response.Id == connectionID {
			result := *response.Result
			remoteDescription = response.Result
			if err := peerConnection.SetRemoteDescription(result); err != nil {
				log.Fatal(err)
			}
		}else if response.Id != 0 && response.Method == "offer"{
			peerConnection.SetRemoteDescription(*response.Params)
			answer, err := peerConnection.CreateAnswer(nil)

			if err != nil {
				log.Fatal(err)
			}

			peerConnection.SetLocalDescription(answer)

			connectionUUID := uuid.New()
			connectionID = uint64(connectionUUID.ID())

			offerJSON, err := json.Marshal(&SendAnswer{
				SID: "test room",
				Answer: peerConnection.LocalDescription(),
			})

			params := (*json.RawMessage)(&offerJSON)

			answerMessage := jsonrpc2.Request{
				Method: "answer",
				Params: params,
				ID: jsonrpc2.ID{
					IsString: false,
					Str: "",
					Num: connectionID,
				},
			}

			reqBodyBytes := new(bytes.Buffer)
			json.NewEncoder(reqBodyBytes).Encode(answerMessage)
			
			responseBytes := reqBodyBytes.Bytes()
			c.WriteMessage(websocket.TextMessage, responseBytes)
		}else if response.Method == "trickle"{
			var trickleResponse TrickleResponse

			if err := json.Unmarshal(message, &trickleResponse); err != nil {
				log.Fatal(err)
			}

			err := peerConnection.AddICECandidate(*trickleResponse.Params.Candidate)

			if err != nil {
				log.Fatal(err)
			}
		}
	}
}