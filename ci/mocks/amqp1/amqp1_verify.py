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
    'standalone_check1': {'output': 'foobar\n', 'status': 2},
    'standalone_check2': {'output': 'woobalooba\n', 'status': 0}
}


def timeout_handler(signum, frame):
    print("Verification timed out")
    sys.exit(2)


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
        print("RECEIVE: Created receiver for source address '{0}'".format
              (self.address))

    def on_message(self, event):
        if self.received < self.expected:
            try:
                result = json.loads(event.message.body)
                print(f"Verifying check result {result['check']['name']}")
                assert(result['check']['name'] in CI_TEST_RESULTS)
                print("Result name found in list of expected results.")

                for test in ("status", "output"):
                    assert(CI_TEST_RESULTS[result['check']['name']][test] == result['check'][test])
                    print(f"successful verification of {test} in result.")

                if result['check']['name'] not in self.hits:
                    self.received += 1
                self.hits.add(result['check']['name'])
            except AssertionError as ex:
                print(f"Failed verification of {test} in result: {result}")
                sys.exit(1)

            if self.received == self.expected:
                print("Verified!")
                event.receiver.close()
                event.connection.close()
                if self.hits != set(CI_TEST_RESULTS.keys()):
                    print("Failed verification. Not all expected messages "
                          "were received.")
                    sys.exit(1)


@click.command()
@click.option('--amqp-url', required=True, default='amqp://127.0.0.1:5666/collectd/checks')
@click.option('--timeout', type=int, required=True, default=10)
def main(amqp_url, timeout):
    trash, url = amqp_url.split('//', 1)
    trash, address = url.split('/', 1)
    container = Container(Verifier(amqp_url, address, timeout))
    container.run()


if __name__ == '__main__':
    main()
