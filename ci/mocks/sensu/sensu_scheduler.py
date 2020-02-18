#!/usr/bin/env python

"""
Mocked Ruby based Sensu server. Connects to the RabbitMQ instance and sends
check requests as Sensu server would and waits for appropriate response.
Fails the response is not received at all or in invalid format.
"""

import click
import datetime
import json
import pika
import time


CI_SUBSCRIPTIONS = ['all', 'citest']
CI_TEST_REQUESTS = [
    {'name': 'test1', 'command': 'echo "foo" && exit 0',
     'issued': int(datetime.datetime.utcnow().timestamp())},
    {'name': 'test2', 'command': 'echo "bar" && exit 1',
     'issued': int(datetime.datetime.utcnow().timestamp())},
    {'name': 'test3', 'command': 'echo "baz" && exit 2',
     'issued': int(datetime.datetime.utcnow().timestamp())},
]


@click.command()
@click.option('--rabbit-url', required=True,
              default='amqp://guest:guest@127.0.0.1:5672/%2fsensu')
def main(rabbit_url):
    connection = pika.BlockingConnection(pika.URLParameters(rabbit_url))
    channel = connection.channel()

    # Declare all the exchanges and queues Sensu is using
    for exchange in CI_SUBSCRIPTIONS:
        channel.exchange_declare(exchange=exchange, exchange_type='fanout')
    for queue in ['results', 'keepalives']:
        channel.queue_declare(queue=queue, durable=True, exclusive=False, auto_delete=False)
    # Leave some time to collectd-sensubility to connect
    time.sleep(20)

    # Send test check requests
    channel.confirm_delivery()
    # Hold on to keep created exchanges
    while True:
        for request in CI_TEST_REQUESTS:
            props = pika.BasicProperties(content_type='application/json')
            channel.basic_publish(exchange='citest', routing_key='citest',
                                  body=json.dumps(request), properties=props)
        time.sleep(30)


if __name__ == '__main__':
    main()
