#!/usr/bin/env python

""",
Mocked AMQP1.0 server side. Connects to the QDR instance and waits
for appropriate response. Fails when the response is not received at all
or in invalid format.
"""

import click
import json
import signal
import sys

from proton.handlers import MessagingHandler
from proton.reactor import Container


CI_TEST_RESULTS = {
    'standalone_check1': {
        "labels": {
            "check": "standalone_check1",
            "severity": "FAILURE",
        },
        "annotations": {
            "command": "echo 'foobar' \u0026\u0026 exit 2",
            "output": 'foobar\n',
            "status": 2
        }
    },
    'standalone_check2': {
        "labels": {
            "check": "standalone_check2",
            "severity": "OKAY"
        },
        "annotations": {
            "command": "echo 'woobalooba' \u0026\u0026 exit 0",
            "output": "woobalooba\n",
            "status": 0
        }
    },

}


def timeout_handler(signum, frame):
    print("Verification timed out")
    sys.exit(2)


def verify_deeply(addr, the_dict, value):
    parts = addr.split('.', 1)
    if parts[0] not in the_dict:
        print(f"Expected key {parts[0]} was not found "
              f"in received message: {the_dict}")
        sys.exit(1)
    if len(parts) == 1:
        assert(the_dict[addr] == value)
    else:
        verify_deeply('.'.join(parts[1:]), the_dict[parts[0]], value)


class Verifier(MessagingHandler):
    def __init__(self, url, address, timeout):
        super(Verifier, self).__init__()
        self.url = url
        self.address = address
        self.expected = len(CI_TEST_RESULTS)
        self.received = 0
        self.hits = set()
        self.timeout = timeout

    def on_start(self, event):
        print(f'Connecting to {self.url} for listening on {self.address}.')
        conn = event.container.connect(self.url)
        event.container.create_receiver(conn, self.address)
        # event.container.create_receiver(self.url)
        signal.signal(signal.SIGALRM, timeout_handler)
        signal.alarm(self.timeout)

    def on_link_opened(self, event):
        print("Created receiver for source address '{0}'".format
              (self.address))

    def on_message(self, event):
        try:
            result = json.loads(event.message.body)
            if "labels" in result and "check" in result["labels"] and \
                    result["labels"]["check"] in CI_TEST_RESULTS:
                check_name = result["labels"]["check"]
                self.hits.add(check_name)

                template = CI_TEST_RESULTS[check_name]
                for main_key in ("labels", "annotations"):
                    for key, item in template[main_key].items():
                        verify_deeply(f"{main_key}.{key}", result, item)
                self.received += 1
            else:
                main_key, key, item = "labels", "check", "<N/A>"
                raise AssertionError()
        except AssertionError as ex:
            print(f"Failed verification of {main_key}.{key} "
                  f"(expected {item}) in result: {result}")
            sys.exit(1)

        if self.received >= self.expected:
            print("Verified!")
            event.receiver.close()
            event.connection.close()
            if self.hits != set(CI_TEST_RESULTS.keys()):
                print("Failed verification. Not all expected messages "
                      "were received.")
                sys.exit(1)


@click.command()
@click.option('--amqp-url', required=True, default='amqp://127.0.0.1:5666/collectd/events')
@click.option('--timeout', type=int, required=True, default=10)
def main(amqp_url, timeout):
    trash, url = amqp_url.split('//', 1)
    trash, address = url.split('/', 1)
    container = Container(Verifier(amqp_url, address, timeout))
    container.run()


if __name__ == '__main__':
    main()
