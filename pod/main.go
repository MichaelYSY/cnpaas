package main

import (
	"flag"
	"fmt"
	"github.com/afex/hystrix-go/hystrix"
	"github.com/asim/go-micro/plugins/registry/consul/v3"
	ratelimit "github.com/asim/go-micro/plugins/wrapper/ratelimiter/uber/v3"
	opentracing2 "github.com/asim/go-micro/plugins/wrapper/trace/opentracing/v3"
	"github.com/asim/go-micro/v3"
	"github.com/asim/go-micro/v3/registry"
	"github.com/asim/go-micro/v3/server"
	"github.com/jinzhu/gorm"
	"github.com/michaelysy/cnpass/common"
	"github.com/michaelysy/cnpass/pod/domain/repository"
	service2 "github.com/michaelysy/cnpass/pod/domain/service"
	"github.com/michaelysy/cnpass/pod/handler"
	hystrix2 "github.com/michaelysy/cnpass/pod/plugin/hystrix"
	"github.com/michaelysy/cnpass/pod/proto/pod"
	"github.com/opentracing/opentracing-go"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"net"
	"net/http"
	"path/filepath"
	"strconv"

	_ "github.com/jinzhu/gorm/dialects/mysql"
)

var (
	hostIp               = "172.19.8.193"
	serviceHost          = hostIp
	servicePort          = "8081"
	consulHost           = hostIp
	consulPort     int64 = 8500
	tracerHost           = hostIp
	tracerPort           = 6831
	hystrixPort          = 9091
	prometheusPort       = 9191
)

func main() {

	// 1. Registration Center
	consul := consul.NewRegistry(func(options *registry.Options) {
		options.Addrs = []string{
			consulHost + ":" + strconv.FormatInt(consulPort, 10),
		}
	})

	// 2. Configuration Center
	consulConfig, err := common.GetConsulConfig(consulHost, consulPort, "/micro/config")
	if err != nil {
		common.Error(err)
	}

	// 3. Connect and initialize MySQL database
	mysqlInfo := common.GetMysqlFromConsul(consulConfig, "mysql")
	db, err := gorm.Open("mysql", mysqlInfo.User+":"+mysqlInfo.Pwd+"@("+mysqlInfo.Host+":3306)/"+mysqlInfo.Database+"?charset=utf8&parseTime=True&loc=Local")
	if err != nil {
		fmt.Println(err)
		common.Error(err)
	}
	defer db.Close()
	db.SingularTable(true)

	// 4. Tracing
	t, io, err := common.NewTracer("go.micro.service.pod", tracerHost+":"+strconv.Itoa(tracerPort))
	if err != nil {
		common.Error(err)
	}
	defer io.Close()
	opentracing.SetGlobalTracer(t)

	// 5. 熔断器
	hystrixStreamHandler := hystrix.NewStreamHandler()
	hystrixStreamHandler.Start()
	// 添加监听程序
	go func() {
		//http://172.19.8.193:9092/turbine/turbine.stream
		//看板访问地址 http://127.0.0.1:9002/hystrix，url后面一定要带 /hystrix
		err = http.ListenAndServe(net.JoinHostPort("0.0.0.0", strconv.Itoa(hystrixPort)), hystrixStreamHandler)
		if err != nil {
			common.Error(err)
		}
	}()

	// 6. 日志中心
	// 1）需要程序日志打入到日志文件中 2）在程序中添加filebeat.yml文件 3）启动filebeat，启动命令 ./filebeat -e -c filebeat.yml
	fmt.Println("日志统一记录在根目录 micro.log 文件中，请点击查看日志！")

	// 7. 添加监控
	common.PrometheusBoot(prometheusPort)

	// 8. 创建K8s连接
	// 在集群外部使用
	// -v /Users/cap/.kube/config:/root/.kube/config
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "kubeconfig file 在当前系统中的地址")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "kubeconfig file 在当前系统中的地址")
	}
	flag.Parse()
	// 创建 config 实例
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		common.Fatal(err.Error())
	}

	// 在集群中使用
	// config , err := rest.InClusterConfig()
	// if err!=nil {
	// 	panic(err.Error())
	// }

	// 创建程序可操作的客户端
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		common.Fatal(err.Error())
	}

	// 9. 创建服务实例
	service := micro.NewService(
		micro.Server(server.NewServer(func(options *server.Options) {
			options.Advertise = serviceHost + ":" + servicePort
		})),
		micro.Name("go.micro.service.pod"),
		micro.Version("latest"),
		micro.Address(":"+servicePort),
		micro.Registry(consul),
		micro.WrapHandler(opentracing2.NewHandlerWrapper(opentracing.GlobalTracer())),
		micro.WrapClient(opentracing2.NewClientWrapper(opentracing.GlobalTracer())),
		micro.WrapClient(hystrix2.NewClientHystrixWrapper()),
		micro.WrapHandler(ratelimit.NewHandlerWrapper(1000)),
	)

	// 10. 初始化服务
	service.Init()

	// 11. 初始化数据表，只能初始化一次
	// err = repository.NewPodRepository(db).InitTable()
	// if err != nil {
	// 	common.Fatal(err)
	// }

	// 12. 注册句柄
	podDataService := service2.NewPodDataService(repository.NewPodRepository(db), clientset)
	pod.RegisterPodHandler(service.Server(), &handler.PodHandler{PodDataService: podDataService})

	// 13. 启动服务
	if err := service.Run(); err != nil {
		common.Fatal(err)
	}

}
