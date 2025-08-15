**SDI-12 Noise Mitigation Guidelines**

This document provides best practices and practical tips for reducing electrical noise on an SDI-12 bus. These recommendations are drawn from field experience, community discussions, and guidance from both device documentation and AI-assisted troubleshooting.

---

## 1. Overview of SDI-12 Noise Issues

- **Symptoms**: Unparseable or corrupted sensor readings, intermittent communication failures.
- **Common Causes**: Long cable runs, lack of proper pull-up resistors, power-supply noise coupling into data lines, ground loops, improper shielding.

---

## 2. Pull-Up Resistors

- **Internal Pull-Ups**: Many microcontrollers (e.g., Arduino) include internal pull-up resistors on digital pins. According to the Arduino documentation, these resistances typically range from 20kΩ to 50kΩ and are enabled via software configuration (e.g., `pinMode(pin, INPUT_PULLUP)`).
    - Documentation: https://docs.arduino.cc/learn/microcontrollers/digital-pins/
- **External Pull-Ups**: For longer bus lengths or noisy environments, consider adding external pull-up resistors (e.g., 4.7kΩ to 10kΩ) between the SDI-12 data line and the supply voltage to strengthen the idle-line bias.

---

## 3. Cable Management

- **Shorten Cable Lengths**: The SDI-12 specification supports up to 60m under ideal conditions, but practical setups often benefit from much shorter runs. Minimize cable length wherever possible.
- **Twisted Pair & Shielding**: Use shielded, twisted-pair cable for the SDI-12 data and ground conductors. Connect the shield to earth ground at one end only.
- **Routing**: Keep SDI-12 cables away from high-current power lines, motors, and switching supplies to prevent electromagnetic interference (EMI).

---

## 4. Power Supply & Decoupling

- **Decoupling Capacitors**: Install a 0.1μF ceramic capacitor between VCC and GND at the power input terminals of each SDI-12 device. This helps absorb high-frequency noise and stabilize the supply voltage locally.
- **Common Power Source (Case Study)**: A field test used a Waveshare PoE HAT (C) module to power the Raspberry Pi, Arduino, SE200 sensor, and PLS-C logger simultaneously from a single 12V output pin.
    - Result: Virtually no SDI-12 noise observed under this configuration.
    - Module details: https://www.waveshare.com/wiki/PoE_HAT_(C)

---

## 5. Field Experience & Community Insights

- **GitHub Discussion**: An issue comment on the EnviroDIY Arduino-SDI-12 repository highlights noise on the bus and suggests hardware improvements: https://github.com/EnviroDIY/Arduino-SDI-12/issues/76#issuecomment-844379075
- **AI Recommendations**: Further suggestions from an AI consultation (ChatGPT) include:
    - Emphasize cable-length reduction.
    - Use decoupling capacitors at each device's power input.

---

## 6. Summary of Recommendations

1. **Enable or add pull-up resistors** (internal or external).
2. **Shorten cable runs** and use shielded, twisted-pair wiring.
3. **Install decoupling capacitors** (0.1μF) at each device’s VCC–GND.
4. **Power devices from a common, clean supply** (e.g., PoE HAT).
5. **Isolate routing** away from noisy power or signal cables.

Implementing a combination of these measures will greatly improve SDI-12 communication reliability in the field.
