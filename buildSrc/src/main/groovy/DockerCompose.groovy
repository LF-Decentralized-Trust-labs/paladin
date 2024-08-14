import org.gradle.api.tasks.Exec
import org.gradle.process.ExecResult

class DockerCompose extends Exec {
    DockerCompose() {
        def String composeCommand
        def dockerComposeV2Check = "docker compose version".execute().text
        if (dockerComposeV2Check.contains("Docker Compose")){
            executable 'docker'
            args 'compose'
            composeCommand = 'docker compose'
        } else {
            executable 'docker-compose'
            composeCommand = 'docker-compose'
        }
        args '-f'
        args 'docker-compose-test.yml'

        ignoreExitValue true

        doLast {
            def ExecResult execResult = getExecutionResult().get();
            if(execResult.getExitValue() != 0) {
                println "${composeCommand} -f ${getProject().projectDir}/docker-compose-test.yml logs".execute().waitForProcessOutput(System.err,System.err)
                throw new Exception("Docker compose failed");
            }
        }

    }
}
