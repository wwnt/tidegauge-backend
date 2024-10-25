# 1. Flash Arduino

1. Install Arduino Ide.
2. Open arduino/arduino.ino
3. Connect arduino via usb cable
4. Click upload

# 2. Supported functions

- Sdi-12
- Read Analog Voltage

# 3. How to use

0xFF is used to separate.

The sdi-12 command should be followed by one byte of extraWakeTime for the second parameter of `sendCommand`.
(Check out the documentation here https://envirodiy.github.io/Arduino-SDI-12/class_s_d_i12.html)

## 3.1 Examples

### 3.1.1. Send sdi-12 command: ?!

    3F 21 00 FF

### 3.1.2. Read A0 pin voltage.

    0E FF
