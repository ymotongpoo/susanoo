# Copyright 2019 Yoshi Yamaguchi
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
#     http://www.apache.org/licenses/LICENSE-2.0
# 
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

TARGET=susanoo
USER=pi
HOST=pimoroni2.local

.PHONY: clean deploy

build: main.go
	go build -o $(TARGET) -ldflags \
		"-X main.OWMAPIKey=$(OWM_API_KEY) -X main.DarkSkyAPIKey=$(DARK_SKY_API_KEY)"

armbuild: main.go
	GOOS=linux GOARCH=arm GOARM=6 go build -o $(TARGET) -ldflags \
		"-X main.OWMAPIKey=$(OWM_API_KEY) -X main.DarkSkyAPIKey=$(DARK_SKY_API_KEY)"

clean:
	go clean
	rm -rf $(TARGET)

deploy: clean armbuild
	scp $(TARGET) $(USER)@$(HOST):~/susanoo/$(TARGET)