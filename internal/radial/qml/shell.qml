import QtQuick
import Quickshell
import Quickshell.Io
import Quickshell.Wayland

ShellRoot {
    id: root

    property var modes: []
    property int selectedIndex: 0
    property string preferred: ""
    property bool dataLoading: true
    readonly property color accent: "#ffffff"
    readonly property var selectedMode: modes.length > 0 ? modes[selectedIndex] : null

    function angleFor(index) {
        if (root.modes.length === 0)
            return -90;

        return (360 * index) / root.modes.length - 90;
    }

    function moveSelection(delta) {
        if (root.modes.length === 0)
            return ;

        root.selectedIndex = (root.selectedIndex + delta + root.modes.length) % root.modes.length;
    }

    function selectPreferred(name) {
        if (!name)
            return ;

        for (let i = 0; i < root.modes.length; i++) {
            if ((root.modes[i].name || "") === name) {
                root.selectedIndex = i;
                return ;
            }
        }
    }

    function shellQuote(value) {
        return "'" + String(value).replace(/'/g, "'\"'\"'") + "'";
    }

    function writeResult(action, name) {
        const cleanedName = String(name || "").trim();
        if (cleanedName.length === 0)
            return ;

        writer.command = ["bash", "-lc", "printf '%s\\t%s\\n' " + root.shellQuote(action) + " " + root.shellQuote(cleanedName) + " > \"$CRUSADE_RADIAL_OUT\""];
        writer.running = true;
    }

    function handleKey(event) {
        if (event.key === Qt.Key_Escape || (event.key === Qt.Key_BracketLeft && (event.modifiers & Qt.ControlModifier))) {
            Qt.quit();
            event.accepted = true;
            return ;
        }
        if (event.key === Qt.Key_Left || event.key === Qt.Key_H || event.key === Qt.Key_K) {
            root.moveSelection(-1);
            event.accepted = true;
            return ;
        }
        if (event.key === Qt.Key_Right || event.key === Qt.Key_L || event.key === Qt.Key_J) {
            root.moveSelection(1);
            event.accepted = true;
            return ;
        }
        if (event.key === Qt.Key_Return || event.key === Qt.Key_Enter || event.key === Qt.Key_Space) {
            if (root.selectedMode)
                root.writeResult("select", root.selectedMode.name || "");

            event.accepted = true;
            return ;
        }
        if (event.key >= Qt.Key_1 && event.key <= Qt.Key_9) {
            const index = event.key - Qt.Key_1;
            if (index < root.modes.length)
                root.writeResult("select", root.modes[index].name || "");

            event.accepted = true;
        }
    }

    Component.onCompleted: dataLoader.running = true

    Process {
        id: dataLoader

        running: false
        command: ["bash", "-lc", "cat \"$CRUSADE_RADIAL_DATA\""]

        stdout: StdioCollector {
            onStreamFinished: {
                try {
                    const parsed = JSON.parse(this.text);
                    root.modes = parsed.modes || [];
                    root.preferred = parsed.preferred || "";
                    root.selectPreferred(root.preferred);
                } catch (error) {
                    console.warn("Unable to parse crusade modes:", error);
                    root.modes = [];
                }
                root.dataLoading = false;
            }
        }

    }

    Process {
        id: writer

        running: false
        onExited: Qt.quit()
    }

    PanelWindow {
        id: panel

        readonly property real shortSide: Math.min(width, height)
        readonly property real orbitRadius: Math.max(154, Math.min(shortSide * 0.31, 270))
        readonly property real tileSize: Math.max(70, Math.min(shortSide * 0.12, 92))
        readonly property real stageSize: Math.max(440, orbitRadius * 2 + tileSize * 2 + 84)

        exclusiveZone: 0
        color: "transparent"
        WlrLayershell.layer: WlrLayer.Overlay
        WlrLayershell.keyboardFocus: WlrKeyboardFocus.Exclusive
        WlrLayershell.namespace: "crusade-radial"

        anchors {
            top: true
            bottom: true
            left: true
            right: true
        }

        Item {
            anchors.fill: parent
            focus: true
            Keys.onPressed: (event) => {
                return root.handleKey(event);
            }
            Component.onCompleted: forceActiveFocus()
        }

        MouseArea {
            anchors.fill: parent
            onClicked: Qt.quit()
        }

        Item {
            id: stage

            width: panel.stageSize
            height: panel.stageSize
            anchors.centerIn: parent

            RingField {
                anchors.fill: parent
                orbitRadius: panel.orbitRadius
                accent: root.accent
                focusAngle: root.angleFor(root.selectedIndex)
            }

            Column {
                anchors.centerIn: parent
                width: Math.min(420, parent.width * 0.58)
                spacing: 12

                Text {
                    width: parent.width
                    horizontalAlignment: Text.AlignHCenter
                    text: "CRUSADE"
                    color: "#707070"
                    font.family: "JetBrainsMono Nerd Font"
                    font.pixelSize: 12
                    font.weight: Font.Bold
                }

                Text {
                    width: parent.width
                    horizontalAlignment: Text.AlignHCenter
                    text: root.dataLoading ? "CRUSADE" : (root.selectedMode ? String(root.selectedMode.label || root.selectedMode.name).toUpperCase() : "CRUSADE")
                    color: root.accent
                    elide: Text.ElideRight
                    font.family: "JetBrainsMono Nerd Font"
                    font.pixelSize: 27
                    font.weight: Font.Bold
                }

                Rectangle {
                    anchors.horizontalCenter: parent.horizontalCenter
                    width: 88
                    height: 2
                    color: root.accent
                    opacity: 0.88
                }

                Text {
                    width: parent.width
                    horizontalAlignment: Text.AlignHCenter
                    text: root.selectedMode ? (root.selectedMode.blurb || "") : ""
                    color: "#707070"
                    elide: Text.ElideMiddle
                    font.family: "JetBrainsMono Nerd Font"
                    font.pixelSize: 14
                }

                Text {
                    width: parent.width
                    horizontalAlignment: Text.AlignHCenter
                    text: "enter opens    esc closes"
                    color: "#707070"
                    font.family: "JetBrainsMono Nerd Font"
                    font.pixelSize: 10
                }

            }

            Repeater {
                model: root.modes

                OrbitTile {
                    required property var modelData
                    required property int index
                    readonly property real angle: root.angleFor(index) * Math.PI / 180

                    appName: modelData.label || modelData.name || ""
                    accent: root.accent
                    selected: index === root.selectedIndex
                    number: index + 1
                    showLabel: true
                    tileSize: panel.tileSize
                    x: stage.width / 2 - width / 2 + Math.cos(angle) * panel.orbitRadius
                    y: stage.height / 2 - height / 2 + Math.sin(angle) * panel.orbitRadius

                    MouseArea {
                        anchors.fill: parent
                        hoverEnabled: true
                        cursorShape: Qt.PointingHandCursor
                        onEntered: root.selectedIndex = parent.index
                        onClicked: root.writeResult("select", parent.modelData.name || "")
                    }

                    Behavior on x {
                        NumberAnimation {
                            duration: 180
                            easing.type: Easing.OutCubic
                        }

                    }

                    Behavior on y {
                        NumberAnimation {
                            duration: 180
                            easing.type: Easing.OutCubic
                        }

                    }

                }

            }

        }

        Row {
            spacing: 8

            anchors {
                bottom: parent.bottom
                horizontalCenter: parent.horizontalCenter
                bottomMargin: 30
            }

            Repeater {
                model: root.modes.length

                Rectangle {
                    required property int index

                    width: index === root.selectedIndex ? 32 : 12
                    height: 4
                    color: index === root.selectedIndex ? root.accent : Qt.rgba(1, 1, 1, 0.22)

                    Behavior on width {
                        NumberAnimation {
                            duration: 160
                            easing.type: Easing.OutCubic
                        }

                    }

                }

            }

        }

    }

}
