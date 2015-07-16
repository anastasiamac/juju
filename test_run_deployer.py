from argparse import Namespace
import logging
import os
import stat
import subprocess
from tempfile import NamedTemporaryFile
from unittest import TestCase

from mock import patch

from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    )
from run_deployer import (
    check_health,
    parse_args,
    run_deployer
    )


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(['/bundle/path', 'test_env', 'new/bin/juju',
                           '/tmp/logs', 'test_job'])
        self.assertEqual(args.bundle_path, '/bundle/path')
        self.assertEqual(args.env, 'test_env')
        self.assertEqual(args.juju_bin, 'new/bin/juju')
        self.assertEqual(args.logs, '/tmp/logs')
        self.assertEqual(args.temp_env_name, 'test_job')
        self.assertEqual(args.bundle_name, None)
        self.assertEqual(args.health_cmd, None)
        self.assertEqual(args.keep_env, False)
        self.assertEqual(args.agent_url, None)
        self.assertEqual(args.agent_stream, None)
        self.assertEqual(args.series, None)
        self.assertEqual(args.debug, False)
        self.assertEqual(args.verbose, logging.INFO)


class TestRunDeployer(TestCase):

    def test_run_deployer(self):
        with patch('run_deployer.boot_context'):
            with patch('run_deployer.SimpleEnvironment.from_config',
                       return_value=SimpleEnvironment('bar')) as env:
                with patch('run_deployer.EnvJujuClient.by_version',
                           return_value=EnvJujuClient(env, '1.234-76', None)):
                    with patch('run_deployer.parse_args',
                               return_value=Namespace(
                                   temp_env_name='foo', env='bar', series=None,
                                   agent_url=None, agent_stream=None,
                                   juju_bin='', logs=None, keep_env=False,
                                   health_cmd=None, debug=False,
                                   bundle_path='', bundle_name='',
                                   verbose=logging.INFO)):
                        with patch(
                                'run_deployer.EnvJujuClient.deployer') as dm:
                            with patch('run_deployer.check_health') as hm:
                                run_deployer()
        self.assertEqual(dm.call_count, 1)
        self.assertEqual(hm.call_count, 0)

    def test_run_deployer_health(self):
        with patch('run_deployer.boot_context'):
            with patch('run_deployer.SimpleEnvironment.from_config',
                       return_value=SimpleEnvironment('bar')) as env:
                with patch('run_deployer.EnvJujuClient.by_version',
                           return_value=EnvJujuClient(env, '1.234-76', None)):
                    with patch('run_deployer.parse_args',
                               return_value=Namespace(
                                   temp_env_name='foo', env='bar', series=None,
                                   agent_url=None, agent_stream=None,
                                   juju_bin='', logs=None, keep_env=False,
                                   health_cmd='/tmp/check', debug=False,
                                   bundle_path='', bundle_name='',
                                   verbose=logging.INFO)):
                        with patch('run_deployer.EnvJujuClient.deployer'):
                            with patch('run_deployer.check_health') as hm:
                                run_deployer()
        self.assertEqual(hm.call_count, 1)


class TestIsHealthy(TestCase):

    def test_check_health(self):
        SCRIPT = """#!/bin/bash\necho -n 'PASS'\nexit 0"""
        with NamedTemporaryFile(delete=False) as health_script:
            health_script.write(SCRIPT)
            os.fchmod(health_script.fileno(), stat.S_IEXEC | stat.S_IREAD)
            health_script.close()
            with patch('logging.info') as lo_mock:
                check_health(health_script.name)
            os.unlink(health_script.name)
            self.assertEqual(lo_mock.call_args[0][0],
                             'Health check output: PASS')

    def test_check_health_with_env_name(self):
        SCRIPT = """#!/bin/bash\necho -n \"PASS on $1\"\nexit 0"""
        with NamedTemporaryFile(delete=False) as health_script:
            health_script.write(SCRIPT)
            os.fchmod(health_script.fileno(), stat.S_IEXEC | stat.S_IREAD)
            health_script.close()
            with patch('logging.info') as lo_mock:
                check_health(health_script.name, 'foo')
            os.unlink(health_script.name)
            self.assertEqual(lo_mock.call_args[0][0],
                             'Health check output: PASS on foo')

    def test_check_health_fail(self):
        SCRIPT = """#!/bin/bash\necho -n 'FAIL'\nexit 1"""
        with NamedTemporaryFile(delete=False) as health_script:
            health_script.write(SCRIPT)
            os.fchmod(health_script.fileno(), stat.S_IEXEC | stat.S_IREAD)
            health_script.close()
            with patch('logging.error') as le_mock:
                with self.assertRaises(subprocess.CalledProcessError):
                    check_health(health_script.name)
            os.unlink(health_script.name)
            self.assertEqual(le_mock.call_args[0][0], 'FAIL')

    def test_check_health_with_no_execute_perms(self):
        SCRIPT = """#!/bin/bash\nexit 0"""
        with NamedTemporaryFile(delete=False) as health_script:
            health_script.write(SCRIPT)
            os.fchmod(health_script.fileno(), stat.S_IREAD)
            health_script.close()
            with patch('logging.error') as le_mock:
                with self.assertRaises(OSError):
                    check_health(health_script.name)
            os.unlink(health_script.name)
        self.assertRegexpMatches(
            le_mock.call_args[0][0],
            r'Failed to execute.*: \[Errno 13\].*')
