# ADR 0001: MQTT topic model

Status: accepted. Four topics per station (`cmd`, `telemetry`, `status`, `events`), QoS 1, JSON with `schemaVersion`.
The station name is the topic key; broker ACL (Mosquitto) / IoT policy (AWS) restricts each client certificate to its own `gs/{station}/#`.
