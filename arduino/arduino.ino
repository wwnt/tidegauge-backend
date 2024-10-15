#include <SDI12.h>

#define SERIAL_BAUD 9600 /*!< The baud rate for the output serial port */
#define DATA_PIN 7		 /*!< The pin of the SDI-12 data bus */

/** Define the SDI-12 bus */
SDI12 mySDI12(DATA_PIN);

void setup()
{
	Serial.begin(SERIAL_BAUD, SERIAL_8N1);
	while (!Serial)
		;

	// Initiate serial connection to SDI-12 bus
	mySDI12.begin();
	delay(500); // allow things to settle
}

char serialInByte = 0;
String serialMsgStr = "";

char sdiInByte = 0;
String sdiMsgStr = "";

int sdiAvail = 0;

void loop()
{
	// -- READ SDI-12 DATA --
	// If SDI-12 data is available, keep reading until full message consumed
	// (Normally I would prefer to allow the loop() to keep executing while the string
	//  is being read in--as the serial example above--but SDI-12 depends on very precise
	//  timing, so it is probably best to let it hold up loop() until the string is
	//  complete)
	sdiAvail = mySDI12.available();
	if (sdiAvail > 0)
	{
		while (mySDI12.available())
		{
			sdiInByte = mySDI12.read();
			sdiMsgStr += sdiInByte;
			delay(10); // 1 character ~ 7.5ms
		}
	}
	else if (sdiAvail == 0)
	{
		if (sdiMsgStr != "")
		{
			Serial.print(sdiMsgStr);
			sdiMsgStr = "";
		}
		else if (Serial.available())
		{
			// -- READ SERIAL DATA --
			// If serial data is available, read in a single byte and add it to
			// a String on each iteration
			serialInByte = Serial.read();
			if (serialInByte == '\xFF') // 8 data bits is required
			{
				delay(200);
				if (Serial.available()) // sticky
				{
					serialMsgStr = "";
				}
				else
				{
					if (serialMsgStr.length() > 2 && serialMsgStr[serialMsgStr.length() - 2] == '!')
					{
						int8_t extraWakeTime = serialMsgStr[serialMsgStr.length() - 1];
						serialMsgStr.remove(serialMsgStr.length() - 1);
						// delay(500);
						mySDI12.sendCommand(serialMsgStr, extraWakeTime);
						delay(100); // wait a while for a response
					}
					else if (serialMsgStr.length() == 1)
					{
						Serial.println(analogRead(serialMsgStr[0]));
					}
					serialMsgStr = "";
				}
			}
			else
			{
				serialMsgStr += serialInByte;
			}
		}
	}
	else
	{
		mySDI12.clearBuffer(); // Buffer is full; clear
	}
}