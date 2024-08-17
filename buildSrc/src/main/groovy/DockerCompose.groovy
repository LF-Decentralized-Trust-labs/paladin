import org.gradle.process.ExecResult
import org.gradle.api.DefaultTask

class DockerCompose extends DefaultTask {

    private List<String> _composeFiles = []
    private List<String> _projectName = []
    private List<String> _args = []

    DockerCompose() {
        doFirst {
            this.exec()
        }
    }

    void composeFile(String f) {
        _composeFiles += ['-f', f]
    }

    void projectName(String p) {
        _projectName = ['-p', p]
    }

    void args(Object... args) {
        _args += [*args]
    }

    void dumpLogs(String service = '') {
        if (service == '') {
            println 'Dumping Docker logs'
        } else {
            println "Dumping Docker logs for ${service}"
        }
        new ProcessBuilder().command(*dockerCommand(), 'logs', service)
            .directory(project.projectDir)
            .start()
            .waitForProcessOutput(System.err, System.err)
    }

    private List<String> dockerCommand() {
        if (_composeFiles.size() == 0) {
            _composeFiles = ['-f', 'docker-compose.yml']
        }
        String dockerComposeV2Check = 'docker compose version'.execute().text
        return dockerComposeV2Check.contains('Docker Compose')
            ? ['docker', 'compose', *_composeFiles, *_projectName]
            : ['docker-compose', *_composeFiles, *_projectName]
    }

    protected void exec() {
        List<String> cmd = [*dockerCommand(), *_args]
        ExecResult execResult = project.exec {
            executable cmd.remove(0)
            args cmd
        }
        if (execResult.exitValue != 0) {
            dumpLogs()
        }
        execResult.assertNormalExitValue()
    }

}
